// Package safefile replaces a file whole or not at all: it writes a new file
// beside it and renames it into place, so a crash, a full disk or a failed
// rename never leaves half a file, and never leaves the new copy behind.
package safefile

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Write replaces the file at path with what render writes, with permissions
// perm. The new file is made in path's folder (".<name>.*.tmp", readable by
// the owner only while it is written) and renamed over path, so a link at
// path is replaced, not followed. On any error, render's included, path is
// left as it was and the new file is removed.
func Write(path string, perm fs.FileMode, render func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp") // made 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close() // already closed on the rename path; the first error is the one to report
			_ = os.Remove(tmp)
		}
	}()
	if err := render(f); err != nil {
		return err
	}
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteBytes is Write for data already in memory.
func WriteBytes(path string, perm fs.FileMode, data []byte) error {
	return Write(path, perm, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}
