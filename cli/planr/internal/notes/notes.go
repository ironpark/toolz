package notes

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

// notesRef is the git notes ref planr writes completion records into.
// Notes live outside the commit, so recording one never rewrites history.
const notesRef = plumbing.ReferenceName("refs/notes/planr")

// Note is one completion recorded against a commit.
type Note struct {
	Commit    string
	ShortHash string
	Subject   string
	Plan      string
	Event     string
	Phase     string
	At        string
}

// line is the single line appended to a commit's note. It is written as
// key=value pairs so `planr notes` can read it back without a parser.
func line(planDirectory, event, phase, at string) string {
	fields := []string{
		"planr",
		"plan=" + planDirectory,
		"event=" + event,
	}
	if phase != "" {
		fields = append(fields, "phase="+phase)
	}
	return strings.Join(append(fields, "at="+at), " ")
}

// RecordCompletion links the current HEAD commit to a phase or plan event.
// The state change is already written to disk by the time this runs, so a
// failure here is reported to the caller but must not undo that change.
func RecordCompletion(repoRoot, planDirectory, event string, phaseID int) error {
	repository, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return fmt.Errorf("open repository: %w", err)
	}
	head, err := repository.Head()
	if err != nil {
		return fmt.Errorf("resolve HEAD: %w", err)
	}
	phase := ""
	if phaseID >= 0 {
		phase = fmt.Sprintf("%02d", phaseID)
	}
	entry := line(planDirectory, event, phase, plan.CompletionTimestamp())
	return appendNote(repository, head.Hash(), entry)
}

// appendNote adds a line to the note attached to target, creating the notes
// ref, tree, and note blob when they do not exist yet.
func appendNote(repository *git.Repository, target plumbing.Hash, line string) error {
	notesCommit, err := notesRefCommit(repository)
	if err != nil {
		return err
	}

	entries := []object.TreeEntry{}
	body := line
	if notesCommit != nil {
		tree, err := notesCommit.Tree()
		if err != nil {
			return fmt.Errorf("read notes tree: %w", err)
		}
		for _, entry := range tree.Entries {
			// Keep every other note untouched; replace only the target's entry.
			if noteTargetHash(entry.Name) == target.String() {
				existing, err := readBlob(repository, entry.Hash)
				if err != nil {
					return err
				}
				if trimmed := strings.TrimRight(existing, "\n"); trimmed != "" {
					body = trimmed + "\n\n" + line
				}
				continue
			}
			entries = append(entries, entry)
		}
	}

	blobHash, err := writeBlob(repository, body+"\n")
	if err != nil {
		return err
	}
	entries = append(entries, object.TreeEntry{
		Name: target.String(),
		Mode: filemode.Regular,
		Hash: blobHash,
	})
	// git expects tree entries in name order.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	treeHash, err := writeTree(repository, entries)
	if err != nil {
		return err
	}

	parents := []plumbing.Hash{}
	if notesCommit != nil {
		parents = append(parents, notesCommit.Hash)
	}
	signature := noteSignature(repository)
	commit := &object.Commit{
		Author:       signature,
		Committer:    signature,
		Message:      fmt.Sprintf("planr: note on %s\n", target.String()),
		TreeHash:     treeHash,
		ParentHashes: parents,
	}
	commitHash, err := encodeObject(repository, commit.Encode)
	if err != nil {
		return fmt.Errorf("write notes commit: %w", err)
	}
	if err := repository.Storer.SetReference(plumbing.NewHashReference(notesRef, commitHash)); err != nil {
		return fmt.Errorf("update %s: %w", notesRef, err)
	}
	return nil
}

// notesRefCommit returns the commit the notes ref points at, or nil when no
// note has ever been recorded.
func notesRefCommit(repository *git.Repository) (*object.Commit, error) {
	reference, err := repository.Reference(notesRef, true)
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", notesRef, err)
	}
	commit, err := repository.CommitObject(reference.Hash())
	if err != nil {
		return nil, fmt.Errorf("read notes commit: %w", err)
	}
	return commit, nil
}

// noteTargetHash turns a note entry path into the commit hash it annotates.
// git may store notes fanned out as `ab/cdef...`, so path separators are dropped.
func noteTargetHash(path string) string {
	return strings.ReplaceAll(path, "/", "")
}

// noteSignature identifies planr as the note author, preferring the repository's
// configured identity so notes match the user's other commits.
func noteSignature(repository *git.Repository) object.Signature {
	signature := object.Signature{Name: "planr", Email: "planr@localhost", When: time.Now()}
	settings, err := repository.ConfigScoped(gitconfig.GlobalScope)
	if err != nil {
		return signature
	}
	if name := strings.TrimSpace(settings.User.Name); name != "" {
		signature.Name = name
	}
	if email := strings.TrimSpace(settings.User.Email); email != "" {
		signature.Email = email
	}
	return signature
}

func readBlob(repository *git.Repository, hash plumbing.Hash) (string, error) {
	blob, err := repository.BlobObject(hash)
	if err != nil {
		return "", fmt.Errorf("read note blob: %w", err)
	}
	reader, err := blob.Reader()
	if err != nil {
		return "", fmt.Errorf("read note blob: %w", err)
	}
	defer reader.Close()
	contents, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read note blob: %w", err)
	}
	return string(contents), nil
}

func writeBlob(repository *git.Repository, contents string) (plumbing.Hash, error) {
	encoded := repository.Storer.NewEncodedObject()
	encoded.SetType(plumbing.BlobObject)
	writer, err := encoded.Writer()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("write note blob: %w", err)
	}
	if _, err := io.WriteString(writer, contents); err != nil {
		writer.Close()
		return plumbing.ZeroHash, fmt.Errorf("write note blob: %w", err)
	}
	if err := writer.Close(); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("write note blob: %w", err)
	}
	hash, err := repository.Storer.SetEncodedObject(encoded)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("write note blob: %w", err)
	}
	return hash, nil
}

func writeTree(repository *git.Repository, entries []object.TreeEntry) (plumbing.Hash, error) {
	tree := &object.Tree{Entries: entries}
	hash, err := encodeObject(repository, tree.Encode)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("write notes tree: %w", err)
	}
	return hash, nil
}

func encodeObject(repository *git.Repository, encode func(plumbing.EncodedObject) error) (plumbing.Hash, error) {
	encoded := repository.Storer.NewEncodedObject()
	if err := encode(encoded); err != nil {
		return plumbing.ZeroHash, err
	}
	return repository.Storer.SetEncodedObject(encoded)
}

// Read returns every recorded completion, newest first.
// planFilter limits the result to one plan directory when it is not empty.
func Read(repoRoot, planFilter string) ([]Note, error) {
	repository, err := git.PlainOpenWithOptions(repoRoot, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	notesCommit, err := notesRefCommit(repository)
	if err != nil || notesCommit == nil {
		return nil, err
	}
	tree, err := notesCommit.Tree()
	if err != nil {
		return nil, fmt.Errorf("read notes tree: %w", err)
	}

	var notes []Note
	for _, entry := range tree.Entries {
		target := plumbing.NewHash(noteTargetHash(entry.Name))
		body, err := readBlob(repository, entry.Hash)
		if err != nil {
			continue
		}
		shortHash, subject := commitSummary(repository, target)
		for _, line := range strings.Split(body, "\n") {
			note, ok := parseLine(line)
			if !ok {
				continue
			}
			// Notes record the numbered directory, but every other command
			// accepts the bare plan name too, so both are matched here.
			if planFilter != "" && note.Plan != planFilter && draft.Name(note.Plan) != planFilter {
				continue
			}
			note.Commit, note.ShortHash, note.Subject = target.String(), shortHash, subject
			notes = append(notes, note)
		}
	}

	sort.SliceStable(notes, func(i, j int) bool { return notes[i].At > notes[j].At })
	return notes, nil
}

// commitSummary describes the annotated commit; a note may outlive its commit,
// so a missing object degrades to an abbreviated hash with no subject.
func commitSummary(repository *git.Repository, hash plumbing.Hash) (string, string) {
	short := hash.String()
	if len(short) > 7 {
		short = short[:7]
	}
	commit, err := repository.CommitObject(hash)
	if err != nil {
		return short, ""
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(commit.Message), "\n")
	return short, subject
}

func parseLine(line string) (Note, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 || fields[0] != "planr" {
		return Note{}, false
	}
	note := Note{}
	for _, field := range fields[1:] {
		key, value, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		switch key {
		case "plan":
			note.Plan = value
		case "event":
			note.Event = value
		case "phase":
			note.Phase = value
		case "at":
			note.At = value
		}
	}
	return note, note.Plan != "" && note.Event != ""
}

// WarnFailure reports a note that could not be written without failing the
// command, since the plan or phase is already marked done on disk.
func WarnFailure(err error) {
	fmt.Fprintf(os.Stderr, "warning: completion recorded on disk but not linked to a commit: %v\n", err)
}

func WarnStartFailure(err error) {
	fmt.Fprintf(os.Stderr, "warning: phase start recorded on disk but not linked to a commit: %v\n", err)
}
