package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// fetchTimeout bounds one source's resolution and download. A benchmark that
// hangs on an unreachable host has not measured anything, and the trial's own
// timeout would blame the agent for it.
const fetchTimeout = 2 * time.Minute

// commitSHA recognises a ref that is already an immutable commit, which needs
// no network round-trip to resolve and can be served from cache offline.
var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Resolved is a source that has been reduced to bytes on this disk.
type Resolved struct {
	// Source is what was asked for.
	Source Source
	// Commit is the immutable revision the source resolved to: a commit SHA
	// for a repository, the digest of the bytes for an archive. It is what the
	// cache is keyed by and what the report records, so a run can be
	// reproduced from its own output.
	Commit string
	// Skills are the skill directories the tree turned out to contain.
	Skills []Found
	// Cached reports that nothing was downloaded, which is worth saying: it is
	// the difference between a run that needed the network and one that did not.
	Cached bool
}

// Resolver fetches sources into a cache shared by every trial in a run.
//
// The cache is keyed by resolved commit rather than by the text of the source,
// which is what makes it safe to share: two configurations naming the same
// commit different ways get the same bytes, and a branch that moves produces a
// new key instead of quietly overwriting the old one.
type Resolver struct {
	// Root is the cache directory. Empty means the user cache directory.
	Root string
	// HTTP is the client used for GitHub and archive sources.
	HTTP *http.Client
	// Offline refuses any network access, so a run either uses what is already
	// cached or fails saying what it would have needed.
	Offline bool

	// once guards each commit's directory, so trials running concurrently
	// against the same source download it once rather than racing to fill the
	// same path.
	once sync.Map
}

// DefaultRoot is where fetched skills are cached when no root is configured.
func DefaultRoot() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mohae", "skills"), nil
}

func (r *Resolver) root() (string, error) {
	if r.Root != "" {
		return r.Root, nil
	}
	return DefaultRoot()
}

func (r *Resolver) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return http.DefaultClient
}

// Resolve fetches source if it is not already cached and reports the skills it
// contains. It is safe to call concurrently.
func (r *Resolver) Resolve(ctx context.Context, source Source) (Resolved, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	root, err := r.root()
	if err != nil {
		return Resolved{}, err
	}
	commit, err := r.commitFor(ctx, root, source)
	if err != nil {
		return Resolved{}, err
	}
	dir := filepath.Join(root, string(source.Kind), commit)

	cached := true
	gate, _ := r.once.LoadOrStore(dir, &sync.Once{})
	var fetchErr error
	gate.(*sync.Once).Do(func() {
		if _, err := os.Stat(dir); err == nil {
			return
		}
		cached = false
		fetchErr = r.fetch(ctx, source, commit, dir)
	})
	if fetchErr != nil {
		return Resolved{}, fetchErr
	}
	// Re-checked after the gate: a second caller through an already-run Once
	// learns nothing from it, and a fetch that failed must not look cached.
	if _, err := os.Stat(dir); err != nil {
		return Resolved{}, fmt.Errorf("skill source %s: %w", source, err)
	}

	tree := dir
	if source.Subpath != "" {
		tree, err = safeJoin(dir, source.Subpath)
		if err != nil {
			return Resolved{}, fmt.Errorf("skill source %s: %w", source, err)
		}
		if !isSkill(tree) {
			return Resolved{}, fmt.Errorf("skill source %s: %s has no %s", source, source.Subpath, Manifest)
		}
	}
	found, err := discover(tree)
	if err != nil {
		return Resolved{}, fmt.Errorf("skill source %s: %w", source, err)
	}
	return Resolved{Source: source, Commit: commit, Skills: found, Cached: cached}, nil
}

// commitFor reduces the requested ref to the immutable revision the cache is
// keyed by. A ref that is already a commit needs nothing; anything else is
// asked of the host, and a host that cannot be reached falls back to the last
// answer this machine got, so a laptop off the network still runs.
func (r *Resolver) commitFor(ctx context.Context, root string, source Source) (string, error) {
	if commitSHA.MatchString(source.Ref) {
		return source.Ref, nil
	}
	if source.Kind == KindArchive {
		// An archive URL has no revision to ask about, so the bytes are the
		// identity: the same URL serving different content is a different
		// source, and keying by URL alone would hide that.
		return r.archiveDigest(ctx, root, source)
	}

	pointer := filepath.Join(root, "refs", digest(source.Kind, source.Spec, source.Ref))
	if r.Offline {
		return readPointer(pointer, source)
	}
	var (
		commit string
		err    error
	)
	switch source.Kind {
	case KindGitHub:
		commit, err = r.githubCommit(ctx, source)
	default:
		commit, err = gitCommit(ctx, source)
	}
	if err != nil {
		if remembered, cachedErr := readPointer(pointer, source); cachedErr == nil {
			return remembered, nil
		}
		return "", err
	}
	// Best effort: a pointer that cannot be written costs a network round-trip
	// next time, which is not worth failing a run over.
	if err := os.MkdirAll(filepath.Dir(pointer), 0o755); err == nil {
		_ = os.WriteFile(pointer, []byte(commit), 0o644)
	}
	return commit, nil
}

func readPointer(pointer string, source Source) (string, error) {
	data, err := os.ReadFile(pointer)
	if err != nil {
		return "", fmt.Errorf("skill source %s: no cached revision for this ref and the host was not reachable", source)
	}
	return strings.TrimSpace(string(data)), nil
}

// githubCommit asks GitHub what the ref currently points at. The sha media type
// makes the answer the commit itself rather than a document containing it.
func (r *Resolver) githubCommit(ctx context.Context, source Source) (string, error) {
	ref := source.Ref
	if ref == "" {
		ref = "HEAD"
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", source.Owner, source.Repo, ref)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github.sha")
	// Honoured when set, so a private repository or a rate-limited machine can
	// be given credentials without mohae inventing its own way to hold them.
	if token := githubToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := r.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("skill source %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("skill source %s: resolving %s: %s", source, ref, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 128))
	if err != nil {
		return "", fmt.Errorf("skill source %s: %w", source, err)
	}
	commit := strings.TrimSpace(string(body))
	if !commitSHA.MatchString(commit) {
		return "", fmt.Errorf("skill source %s: resolving %s: unexpected answer %q", source, ref, commit)
	}
	return commit, nil
}

func githubToken() string {
	for _, name := range []string{"MOHAE_GITHUB_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if value := os.Getenv(name); value != "" {
			return value
		}
	}
	return ""
}

// gitCommit resolves a ref on any host git can reach, inheriting whatever
// credentials git is already configured with.
func gitCommit(ctx context.Context, source Source) (string, error) {
	ref := source.Ref
	if ref == "" {
		ref = "HEAD"
	}
	output, err := runGit(ctx, "", "ls-remote", "--exit-code", source.Remote, ref)
	if err != nil {
		return "", fmt.Errorf("skill source %s: resolving %s: %w", source, ref, err)
	}
	commit, _, _ := strings.Cut(strings.TrimSpace(output), "\t")
	if !commitSHA.MatchString(commit) {
		return "", fmt.Errorf("skill source %s: resolving %s: unexpected answer %q", source, ref, commit)
	}
	return commit, nil
}

// archiveDigest downloads the archive to the cache and names it by the digest
// of its bytes. The download is the resolution for this kind, so the file is
// kept for fetch to unpack rather than being fetched a second time.
func (r *Resolver) archiveDigest(ctx context.Context, root string, source Source) (string, error) {
	if r.Offline {
		return "", fmt.Errorf("skill source %s: offline, and an archive cannot be resolved without downloading it", source)
	}
	staged, err := os.CreateTemp("", "mohae-skill-*.archive")
	if err != nil {
		return "", err
	}
	defer os.Remove(staged.Name())
	defer staged.Close()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return "", err
	}
	response, err := r.client().Do(request)
	if err != nil {
		return "", fmt.Errorf("skill source %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("skill source %s: %s", source, response.Status)
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(staged, hash), io.LimitReader(response.Body, maxArchiveBytes+1)); err != nil {
		return "", fmt.Errorf("skill source %s: %w", source, err)
	}
	commit := hex.EncodeToString(hash.Sum(nil))
	if err := staged.Close(); err != nil {
		return "", err
	}
	// Parked under the digest so fetch unpacks these bytes instead of asking
	// the server for them again.
	parked := filepath.Join(root, "archives", commit)
	if err := os.MkdirAll(filepath.Dir(parked), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(staged.Name(), parked); err != nil {
		// Across filesystems rename fails; copying is the fallback.
		if err := copyFile(staged.Name(), parked); err != nil {
			return "", err
		}
	}
	return commit, nil
}

// fetch materialises commit at dir. Everything is built in a temporary sibling
// and moved into place, so a cache entry either does not exist or is complete:
// an interrupted download must not leave a half-tree that later runs treat as
// cached.
func (r *Resolver) fetch(ctx context.Context, source Source, commit, dir string) error {
	if r.Offline {
		return fmt.Errorf("skill source %s: offline, and %s is not cached", source, commit[:12])
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(dir), ".staging-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	switch source.Kind {
	case KindGitHub:
		err = r.fetchGitHub(ctx, source, commit, staging)
	case KindGit:
		err = fetchGit(ctx, source, commit, staging)
	case KindArchive:
		root, rootErr := r.root()
		if rootErr != nil {
			return rootErr
		}
		err = unpack(filepath.Join(root, "archives", commit), source.URL, staging)
	default:
		err = fmt.Errorf("skill source %s: unsupported kind %q", source, source.Kind)
	}
	if err != nil {
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		// Another trial won the race and filled the entry first, which is the
		// same outcome as winning it.
		if _, statErr := os.Stat(dir); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func (r *Resolver) fetchGitHub(ctx context.Context, source Source, commit, dir string) error {
	endpoint := fmt.Sprintf("https://codeload.github.com/%s/%s/tar.gz/%s", source.Owner, source.Repo, commit)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	if token := githubToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := r.client().Do(request)
	if err != nil {
		return fmt.Errorf("skill source %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("skill source %s: downloading %s: %s", source, commit[:12], response.Status)
	}
	staged, err := os.CreateTemp("", "mohae-skill-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	if _, err := io.Copy(staged, io.LimitReader(response.Body, maxArchiveBytes+1)); err != nil {
		staged.Close()
		return fmt.Errorf("skill source %s: %w", source, err)
	}
	if err := staged.Close(); err != nil {
		return err
	}
	return unpack(staged.Name(), "archive.tar.gz", dir)
}

// fetchGit builds a repository holding exactly the wanted commit. The shallow
// fetch is tried first and a full one is the fallback, because a server may
// refuse to serve an arbitrary commit by name.
func fetchGit(ctx context.Context, source Source, commit, dir string) error {
	if _, err := runGit(ctx, "", "init", "--quiet", dir); err != nil {
		return fmt.Errorf("skill source %s: %w", source, err)
	}
	if _, err := runGit(ctx, dir, "remote", "add", "origin", source.Remote); err != nil {
		return fmt.Errorf("skill source %s: %w", source, err)
	}
	if _, err := runGit(ctx, dir, "fetch", "--quiet", "--depth", "1", "origin", commit); err != nil {
		if _, fallback := runGit(ctx, dir, "fetch", "--quiet", "origin"); fallback != nil {
			return fmt.Errorf("skill source %s: fetching %s: %w", source, commit[:12], err)
		}
	}
	if _, err := runGit(ctx, dir, "checkout", "--quiet", commit); err != nil {
		return fmt.Errorf("skill source %s: checking out %s: %w", source, commit[:12], err)
	}
	// The history is not part of the skill and would be copied into every
	// workspace that installs it.
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		return err
	}
	return nil
}

// unpack extracts an archive and strips the single wrapping directory that
// repository hosts add, so the tree is rooted where the repository is rather
// than one level below it.
func unpack(archivePath, name, dir string) error {
	if err := extract(archivePath, name, dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return nil
	}
	wrapper := filepath.Join(dir, entries[0].Name())
	inner, err := os.ReadDir(wrapper)
	if err != nil {
		return err
	}
	for _, entry := range inner {
		if err := os.Rename(filepath.Join(wrapper, entry.Name()), filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(wrapper)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	// Nothing here may stop for input: a benchmark cannot answer a credential
	// prompt, and one left unanswered hangs until the timeout.
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("git is required for this source but was not found")
		}
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func digest(parts ...any) string {
	hash := sha256.New()
	for _, part := range parts {
		fmt.Fprintf(hash, "%v\x00", part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}
