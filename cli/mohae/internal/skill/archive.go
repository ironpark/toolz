package skill

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxArchiveBytes bounds what one entry may expand to. A skill is documentation
// and a few scripts; anything larger is either the wrong URL or an archive
// built to fill the disk of whoever unpacks it, and neither should be allowed
// to take the machine down mid-benchmark.
const maxArchiveBytes = 64 << 20

// extract unpacks an archive into dir, choosing the format from the name. Only
// the two forms a repository host serves are supported, which is also all the
// configuration accepts.
func extract(archivePath, name, dir string) error {
	lowered := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lowered, ".zip"):
		return extractZip(archivePath, dir)
	case strings.HasSuffix(lowered, ".tar.gz"), strings.HasSuffix(lowered, ".tgz"), strings.HasSuffix(lowered, ".tar"):
		return extractTar(archivePath, strings.HasSuffix(lowered, ".tar"), dir)
	}
	return fmt.Errorf("unsupported archive %q", name)
}

func extractTar(archivePath string, plain bool, dir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var stream io.Reader = file
	if !plain {
		unzipped, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		defer unzipped.Close()
		stream = unzipped
	}

	reader := tar.NewReader(stream)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		target, err := safeJoin(dir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(target, reader, os.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
		default:
			// Symlinks, devices and hard links are dropped rather than
			// recreated: a link is the one entry that can point outside the
			// directory after extraction has already checked the path, and a
			// skill has no use for one.
		}
	}
}

func extractZip(archivePath, dir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("reading archive: %w", err)
	}
	defer reader.Close()

	for _, entry := range reader.File {
		target, err := safeJoin(dir, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			continue
		}
		opened, err := entry.Open()
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		err = writeEntry(target, opened, entry.Mode().Perm())
		opened.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(target string, content io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if perm == 0 {
		perm = 0o644
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(content, maxArchiveBytes+1))
	if err != nil {
		return err
	}
	if written > maxArchiveBytes {
		return fmt.Errorf("archive entry %s exceeds %d bytes", filepath.Base(target), maxArchiveBytes)
	}
	return nil
}

// safeJoin places an archive entry under dir, refusing anything that would land
// outside it. An archive is downloaded from somewhere else and unpacked with
// mohae's own permissions, so an entry named ../../.ssh/authorized_keys has to
// be rejected here rather than trusted to be well-meant.
func safeJoin(dir, name string) (string, error) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("archive entry %q is an absolute path", name)
	}
	target := filepath.Join(dir, filepath.FromSlash(name))
	// Compared with the separator appended so that a sibling directory sharing
	// a prefix with dir — dir "skills" and target "skills-evil" — is not read
	// as being inside it.
	if target != dir && !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry %q escapes the destination", name)
	}
	return target, nil
}
