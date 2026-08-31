package skill

import "testing"

func TestParseSourceReadsEveryAcceptedSpelling(t *testing.T) {
	cases := []struct {
		name    string
		spec    string
		ref     string
		subpath string
		want    Source
	}{
		{
			name: "github shorthand",
			spec: "vercel-labs/skills",
			want: Source{Kind: KindGitHub, Owner: "vercel-labs", Repo: "skills"},
		},
		{
			name: "github url",
			spec: "https://github.com/vercel-labs/skills",
			want: Source{Kind: KindGitHub, Owner: "vercel-labs", Repo: "skills"},
		},
		{
			name: "github tree url carries the ref and the subpath",
			spec: "https://github.com/vercel-labs/skills/tree/v1.2.0/skills/commit",
			want: Source{Kind: KindGitHub, Owner: "vercel-labs", Repo: "skills", Ref: "v1.2.0", Subpath: "skills/commit"},
		},
		{
			name: "the .git suffix is not part of the repository name",
			spec: "https://github.com/vercel-labs/skills.git",
			want: Source{Kind: KindGitHub, Owner: "vercel-labs", Repo: "skills"},
		},
		{
			name: "scp-like remote",
			spec: "git@gitlab.com:org/repo.git",
			ref:  "main",
			want: Source{Kind: KindGit, Remote: "git@gitlab.com:org/repo.git", Ref: "main"},
		},
		{
			name: "a non-github host is cloned",
			spec: "https://gitlab.com/org/repo",
			want: Source{Kind: KindGit, Remote: "https://gitlab.com/org/repo"},
		},
		{
			name: "archive url",
			spec: "https://example.test/bundle.tar.gz",
			want: Source{Kind: KindArchive, URL: "https://example.test/bundle.tar.gz"},
		},
		{
			name: "an archive on github is still an archive",
			spec: "https://github.com/o/r/archive/refs/tags/v1.zip",
			want: Source{Kind: KindArchive, URL: "https://github.com/o/r/archive/refs/tags/v1.zip"},
		},
		{
			name:    "explicit fields are kept",
			spec:    "owner/repo",
			ref:     "abc",
			subpath: "/skills/one/",
			want:    Source{Kind: KindGitHub, Owner: "owner", Repo: "repo", Ref: "abc", Subpath: "skills/one"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := ParseSource(testCase.spec, testCase.ref, testCase.subpath)
			if err != nil {
				t.Fatalf("ParseSource(%q) = %v", testCase.spec, err)
			}
			want := testCase.want
			want.Spec = testCase.spec
			if got != want {
				t.Errorf("ParseSource(%q) =\n %+v\nwant\n %+v", testCase.spec, got, want)
			}
		})
	}
}

func TestParseSourceRejectsWhatItCannotFetch(t *testing.T) {
	cases := []struct{ name, spec, ref, subpath string }{
		{name: "empty", spec: ""},
		{name: "a local path belongs in path:", spec: "./skills/commit"},
		{name: "an absolute path belongs in path:", spec: "/opt/skills"},
		{name: "a bare word is not a repository", spec: "skills"},
		{name: "github url without a repository", spec: "https://github.com/owner"},
		{name: "an unknown segment after the repository", spec: "https://github.com/o/r/releases/v1/x"},
		{name: "the url ref and ref: disagree", spec: "https://github.com/o/r/tree/main/s", ref: "v2"},
		{name: "the url path and subpath: disagree", spec: "https://github.com/o/r/tree/main/a", subpath: "b"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got, err := ParseSource(testCase.spec, testCase.ref, testCase.subpath); err == nil {
				t.Fatalf("ParseSource(%q) = %+v, want an error", testCase.spec, got)
			}
		})
	}
}

func TestSourceStringNamesTheRefWhenItIsNotAlreadyInTheSpec(t *testing.T) {
	pinned, err := ParseSource("owner/repo", "v1.0.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := pinned.String(); got != "owner/repo@v1.0.0" {
		t.Errorf("String() = %q", got)
	}
	// A /tree/ URL already says which ref it means, so appending it again would
	// read as a second, different pin.
	tree, err := ParseSource("https://github.com/o/r/tree/v1/skills/a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.String(); got != "https://github.com/o/r/tree/v1/skills/a" {
		t.Errorf("String() = %q", got)
	}
}
