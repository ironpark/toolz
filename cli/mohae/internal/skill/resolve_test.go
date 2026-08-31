package skill

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

// githubStub answers the two requests a GitHub source makes: resolving a ref to
// a commit, and downloading that commit's tarball. It counts both so a test can
// assert that a cached source costs no download.
type githubStub struct {
	server    *httptest.Server
	tarball   string
	resolves  atomic.Int32
	downloads atomic.Int32
}

func newGitHubStub(t *testing.T, entries map[string]string) *githubStub {
	t.Helper()
	stub := &githubStub{tarball: tarball(t, entries)}
	stub.server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.Contains(request.URL.Path, "/commits/"):
			stub.resolves.Add(1)
			writer.Write([]byte(testCommit))
		case strings.Contains(request.URL.Path, "/tar.gz/"):
			stub.downloads.Add(1)
			http.ServeFile(writer, request, stub.tarball)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

// client redirects the resolver's two hardcoded hosts at the stub, which is
// what lets the fetching path be exercised without reaching the network.
func (s *githubStub) client() *http.Client {
	base := s.server.URL
	return &http.Client{Transport: rewriteHost{base: base}}
}

type rewriteHost struct{ base string }

func (r rewriteHost) RoundTrip(request *http.Request) (*http.Response, error) {
	redirected := *request
	url := *request.URL
	parsed, err := http.NewRequest(request.Method, r.base+url.Path, nil)
	if err != nil {
		return nil, err
	}
	redirected.URL = parsed.URL
	redirected.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(&redirected)
}

func repoEntries() map[string]string {
	return map[string]string{
		"repo-abc/skills/commit/" + Manifest: "---\nname: commit\n---\n",
		"repo-abc/skills/review/" + Manifest: "---\nname: review\n---\n",
	}
}

func TestResolveFetchesOnceAndServesTheRestFromCache(t *testing.T) {
	stub := newGitHubStub(t, repoEntries())
	resolver := &Resolver{Root: t.TempDir(), HTTP: stub.client()}
	source, err := ParseSource("owner/repo", "main", "")
	if err != nil {
		t.Fatal(err)
	}

	first, err := resolver.Resolve(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached {
		t.Error("the first resolve reported a cache hit")
	}
	if first.Commit != testCommit {
		t.Errorf("commit = %q, want %q", first.Commit, testCommit)
	}
	if len(first.Skills) != 2 {
		t.Fatalf("skills = %+v, want both", first.Skills)
	}

	second, err := resolver.Resolve(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached {
		t.Error("the second resolve downloaded again")
	}
	if got := stub.downloads.Load(); got != 1 {
		t.Errorf("downloads = %d, want 1", got)
	}
}

func TestResolveSkipsTheNetworkEntirelyForACommitPin(t *testing.T) {
	stub := newGitHubStub(t, repoEntries())
	resolver := &Resolver{Root: t.TempDir(), HTTP: stub.client()}
	source, err := ParseSource("owner/repo", testCommit, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	// A ref that is already immutable needs nothing resolved; only the tarball
	// is fetched.
	if got := stub.resolves.Load(); got != 0 {
		t.Errorf("resolves = %d, want 0 for a commit pin", got)
	}
}

func TestResolveSelectsTheSubpathWhenOneIsNamed(t *testing.T) {
	stub := newGitHubStub(t, repoEntries())
	resolver := &Resolver{Root: t.TempDir(), HTTP: stub.client()}
	source, err := ParseSource("owner/repo", "main", "skills/commit")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.Resolve(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Skills) != 1 || resolved.Skills[0].Name != "commit" {
		t.Fatalf("skills = %+v, want just commit", resolved.Skills)
	}
}

func TestResolveRejectsASubpathWithNoManifest(t *testing.T) {
	stub := newGitHubStub(t, repoEntries())
	resolver := &Resolver{Root: t.TempDir(), HTTP: stub.client()}
	source, err := ParseSource("owner/repo", "main", "skills")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), source); err == nil {
		t.Fatal("Resolve() = nil error, want one naming the missing manifest")
	}
}

func TestResolveFallsBackToTheLastKnownCommitWhenTheHostIsUnreachable(t *testing.T) {
	stub := newGitHubStub(t, repoEntries())
	root := t.TempDir()
	source, err := ParseSource("owner/repo", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Resolver{Root: root, HTTP: stub.client()}).Resolve(context.Background(), source); err != nil {
		t.Fatal(err)
	}

	// A resolver whose host answers nothing still runs: the ref was resolved
	// once on this machine and the tree for that commit is already unpacked.
	broken := &Resolver{Root: root, HTTP: &http.Client{Transport: failingTransport{}}}
	resolved, err := broken.Resolve(context.Background(), source)
	if err != nil {
		t.Fatalf("Resolve() = %v, want the remembered commit", err)
	}
	if resolved.Commit != testCommit {
		t.Errorf("commit = %q, want %q", resolved.Commit, testCommit)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, os.ErrDeadlineExceeded
}

func TestResolveOfflineFailsRatherThanFetching(t *testing.T) {
	resolver := &Resolver{Root: t.TempDir(), Offline: true}
	source, err := ParseSource("owner/repo", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), source); err == nil {
		t.Fatal("Resolve() = nil error, want a refusal to reach the network")
	}
}

func TestResolveLeavesNoCacheEntryBehindWhenTheDownloadFails(t *testing.T) {
	root := t.TempDir()
	// Resolves the ref but serves no tarball, so the fetch fails after the
	// commit is known and the entry's path is already decided.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/commits/") {
			writer.Write([]byte(testCommit))
			return
		}
		http.Error(writer, "gone", http.StatusInternalServerError)
	}))
	defer server.Close()

	resolver := &Resolver{Root: root, HTTP: &http.Client{Transport: rewriteHost{base: server.URL}}}
	source, err := ParseSource("owner/repo", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(context.Background(), source); err == nil {
		t.Fatal("Resolve() = nil error, want the download failure")
	}
	// A half-written entry would be treated as cached by every later run.
	if _, err := os.Stat(filepath.Join(root, string(KindGitHub), testCommit)); !os.IsNotExist(err) {
		t.Errorf("a cache entry survived a failed download: %v", err)
	}
}
