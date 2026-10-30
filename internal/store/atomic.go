package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWriteFile replaces path with data so that a reader, or a crash, sees
// either the whole old file or the whole new one. It writes a temporary file in
// the same directory, fsyncs it, then renames it over path. Both the temporary
// path and the final path are checked against the guard first. On failure the
// temporary file is removed and path is untouched.
func (g *Guard) AtomicWriteFile(path string, data []byte, perm os.FileMode) (err error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Errorf("atomic write %q: temp name: %w", path, err)
	}
	tmp := path + ".tmp-" + hex.EncodeToString(suffix[:])
	if err := g.check("write", path, false); err != nil {
		return err
	}
	if err := g.check("write", tmp, false); err != nil {
		return err
	}

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("atomic write %q: create temp file: %w", path, err)
	}
	defer func() {
		if err != nil {
			os.Remove(tmp) // tmp was checked above; best effort
		}
	}()

	if _, err = f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("atomic write %q: write temp file: %w", path, err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("atomic write %q: sync temp file: %w", path, err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("atomic write %q: close temp file: %w", path, err)
	}
	if g.afterTempWrite != nil {
		if err = g.afterTempWrite(tmp); err != nil {
			return fmt.Errorf("atomic write %q: %w", path, err)
		}
	}
	if err = g.Rename(tmp, path); err != nil {
		return fmt.Errorf("atomic write %q: %w", path, err)
	}
	syncDir(filepath.Dir(path))
	return nil
}

// syncDir flushes a directory entry to disk. It is best effort: Windows cannot
// open a directory for sync, and the rename has already happened.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	d.Close()
}
