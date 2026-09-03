package skill

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Kind is how a source has to be fetched. It is decided once, when the source
// is parsed, so the fetching code never has to re-inspect the spelling of a
// spec to know what it is looking at.
type Kind string

const (
	// KindGitHub is a GitHub repository, fetched as a tarball. It is kept
	// apart from KindGit because GitHub can resolve a ref and serve an archive
	// over plain HTTPS, which is faster than a clone and needs no git binary.
	KindGitHub Kind = "github"
	// KindGit is any other repository, fetched with the git CLI. This is the
	// only kind that can reach a private or self-hosted host, since it inherits
	// whatever credentials git is already configured with.
	KindGit Kind = "git"
	// KindArchive is a direct archive URL.
	KindArchive Kind = "archive"
)

// Source is a parsed skill source: where the tree comes from, which revision of
// it is wanted, and which part of it holds the skill.
type Source struct {
	Kind Kind
	// Spec is the text the configuration used, kept verbatim for diagnostics
	// so an error names the source the way its author wrote it.
	Spec string

	// Owner and Repo identify a KindGitHub repository.
	Owner string
	Repo  string
	// Remote is the URL git is given for KindGit.
	Remote string
	// URL is the archive to download for KindArchive.
	URL string

	// Ref is the branch, tag or commit asked for. Empty means the repository's
	// default branch, which is resolved to a commit like any other ref.
	Ref string
	// Subpath is the directory inside the tree that holds the skill. Empty
	// means the tree is searched for skills instead.
	Subpath string
}

// archiveSuffixes are the extensions that make a URL an archive rather than a
// page describing a repository.
var archiveSuffixes = []string{".tar.gz", ".tgz", ".tar", ".zip"}

// shorthand is the `owner/repo` form. Both halves are restricted to the
// characters a host actually allows in a path segment so that a mistyped local
// path — which belongs in `path:` — is reported as an unparseable source
// instead of being fetched from GitHub.
var shorthand = regexp.MustCompile(`^[\w.-]+/[\w.-]+$`)

// scpLike matches git's `user@host:path` remote spelling, which is not a URL
// and so cannot be parsed as one.
var scpLike = regexp.MustCompile(`^[\w.-]+@[\w.-]+:`)

// ParseSource turns a configured source into the form the resolver fetches.
// ref and subpath come from their own configuration fields; a spec that also
// carries them — a /tree/ URL does — fills in whichever field was left empty,
// so the two spellings can be mixed but never silently disagree.
func ParseSource(spec, ref, subpath string) (Source, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Source{}, fmt.Errorf("source is empty")
	}
	source := Source{Spec: spec, Ref: ref, Subpath: strings.Trim(subpath, "/")}

	switch {
	case isLocalPath(spec):
		// Caught before the shorthand pattern, which "./name" would otherwise
		// satisfy with "." as the owner. A local skill belongs in `path:`, and
		// saying so is more use than fetching github.com/./name.
		return Source{}, fmt.Errorf("source %q is a local path: use path: instead", spec)
	case scpLike.MatchString(spec), strings.HasPrefix(spec, "ssh://"), strings.HasPrefix(spec, "git://"):
		source.Kind = KindGit
		source.Remote = spec
		return source, nil
	case shorthand.MatchString(spec):
		owner, repo, _ := strings.Cut(spec, "/")
		source.Kind = KindGitHub
		source.Owner, source.Repo = owner, strings.TrimSuffix(repo, ".git")
		return source, nil
	case strings.HasPrefix(spec, "http://"), strings.HasPrefix(spec, "https://"):
		return parseURL(source, spec)
	}
	return Source{}, fmt.Errorf("unrecognised source %q: expected owner/repo, an https URL, or a git remote", spec)
}

func parseURL(source Source, spec string) (Source, error) {
	parsed, err := url.Parse(spec)
	if err != nil {
		return Source{}, fmt.Errorf("source %q: %w", spec, err)
	}
	lowered := strings.ToLower(parsed.Path)
	for _, suffix := range archiveSuffixes {
		if strings.HasSuffix(lowered, suffix) {
			source.Kind = KindArchive
			source.URL = spec
			return source, nil
		}
	}
	if strings.EqualFold(parsed.Host, "github.com") || strings.EqualFold(parsed.Host, "www.github.com") {
		return parseGitHubURL(source, parsed)
	}
	// Any other host is reached with git, which is the only fetcher that can
	// be pointed at a server whose archive API mohae does not know.
	source.Kind = KindGit
	source.Remote = spec
	return source, nil
}

// parseGitHubURL reads the owner, the repository and — when the URL is the
// /tree/ form a browser produces — the ref and the directory it was pointing
// at. Copying that URL out of the address bar is how a skill is usually found,
// so it is worth understanding rather than asking the author to take it apart.
func parseGitHubURL(source Source, parsed *url.URL) (Source, error) {
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return Source{}, fmt.Errorf("source %q: expected github.com/owner/repo", source.Spec)
	}
	source.Kind = KindGitHub
	source.Owner = segments[0]
	source.Repo = strings.TrimSuffix(segments[1], ".git")

	if len(segments) < 4 {
		return source, nil
	}
	switch segments[2] {
	case "tree", "blob":
	default:
		return Source{}, fmt.Errorf("source %q: expected /tree/<ref>/<path> after the repository", source.Spec)
	}
	// The ref is one segment: a branch name containing a slash cannot be told
	// apart from the directory that follows it without asking the host, and
	// guessing would silently fetch the wrong tree. Such a branch can still be
	// used by naming it in `ref:` instead.
	ref, within := segments[3], path.Join(segments[4:]...)
	if source.Ref == "" {
		source.Ref = ref
	} else if source.Ref != ref {
		return Source{}, fmt.Errorf("source %q names ref %q but ref: says %q", source.Spec, ref, source.Ref)
	}
	if within != "" {
		if source.Subpath == "" {
			source.Subpath = within
		} else if source.Subpath != within {
			return Source{}, fmt.Errorf("source %q names path %q but subpath: says %q", source.Spec, within, source.Subpath)
		}
	}
	return source, nil
}

// isLocalPath reports the spellings that name somewhere on this machine.
func isLocalPath(spec string) bool {
	switch {
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"):
		return true
	case strings.HasPrefix(spec, "/"), strings.HasPrefix(spec, "~"):
		return true
	case spec == ".", spec == "..":
		return true
	}
	return filepath.IsAbs(spec)
}

// String is how a source is named in diagnostics and in the trial result: the
// spec as written, with the ref appended when it was configured separately, so
// two entries differing only by ref do not read as duplicates.
func (s Source) String() string {
	if s.Ref != "" && !strings.Contains(s.Spec, "/tree/") {
		return s.Spec + "@" + s.Ref
	}
	return s.Spec
}
