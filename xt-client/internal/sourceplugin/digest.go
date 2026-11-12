package sourceplugin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// PackageDigest fingerprints package contents for display and copy verification.
// It is not a signature and does not establish publisher identity.
func PackageDigest(dir string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == dir {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("package symlinks are unsupported")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("invalid package file")
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if _, err = hash.Write([]byte(filepath.ToSlash(rel))); err != nil {
			return err
		}
		if _, err = hash.Write([]byte{0}); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
