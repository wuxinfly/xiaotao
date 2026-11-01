package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Keyring struct {
	key []byte
}

func Open(dir string) (*Keyring, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "secret.key")
	if b, err := os.ReadFile(path); err == nil {
		key, err := unprotect(b)
		if err != nil {
			return nil, fmt.Errorf("open keyring: %w", err)
		}
		return &Keyring{key: key}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	blob, err := protect(key)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, ".key-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	if _, err = f.Write(blob); err != nil {
		f.Close()
		os.Remove(name)
		return nil, err
	}
	if err = f.Close(); err != nil {
		os.Remove(name)
		return nil, err
	}
	if err = os.Chmod(name, 0o600); err != nil {
		return nil, err
	}
	if err = os.Rename(name, path); err != nil {
		return nil, err
	}
	return &Keyring{key: key}, nil
}

// OpenReadOnly loads an existing keyring without creating or modifying files.
func OpenReadOnly(dir string) (*Keyring, error) {
	b, err := os.ReadFile(filepath.Join(dir, "secret.key"))
	if err != nil {
		return nil, err
	}
	key, err := unprotect(b)
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}
	return &Keyring{key: key}, nil
}

func (k *Keyring) Encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (k *Keyring) Decrypt(blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("invalid secret blob")
	}
	return gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
}
