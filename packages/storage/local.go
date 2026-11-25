// Package storage implements ports.BlobStore on the local filesystem.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Local stores blobs under a root directory.
type Local struct{ root string }

// NewLocal creates the root directory if needed.
func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("storage.local.mkdir.failed: %w", err)
	}
	return &Local{root: abs}, nil
}

func (l *Local) path(key string) (string, error) {
	p := filepath.Join(l.root, filepath.FromSlash(key))
	if !strings.HasPrefix(p, l.root+string(os.PathSeparator)) {
		return "", errors.New("storage.local.key.invalid")
	}
	return p, nil
}

func (l *Local) Put(_ context.Context, key string, data []byte, _ string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p) //nolint:gosec // path keeps keys inside the root
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
