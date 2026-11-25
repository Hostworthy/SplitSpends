// Package storagetest checks that a blob store keeps the contract of
// ports.BlobStore. Production code never imports it.
package storagetest

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"
)

// Store is the shape of ports.BlobStore, repeated here so that the storage
// adapters' tests need nothing outside the standard library.
type Store interface {
	Put(ctx context.Context, key string, data []byte, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Run checks that store keeps, replaces and removes blobs by key, that
// deleting a missing key succeeds, and that opening one fails with an error
// matching fs.ErrNotExist from Open itself, not from the first read.
func Run(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	key := "receipts/2026/03/a.png"
	read := func() (string, error) {
		t.Helper()
		body, err := store.Open(ctx, key)
		if err != nil {
			return "", err
		}
		defer body.Close()
		data, err := io.ReadAll(body)
		return string(data), err
	}
	for _, data := range []string{"first", "second"} {
		if err := store.Put(ctx, key, []byte(data), "image/png"); err != nil {
			t.Fatal(err)
		}
		if got, err := read(); err != nil || got != data {
			t.Fatalf("want %q back, got %q, %v", data, got, err)
		}
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("deleting a missing key succeeds, got %v", err)
	}
	for _, missing := range []string{key, "receipts/never-stored.png"} {
		body, err := store.Open(ctx, missing)
		if err == nil {
			_ = body.Close()
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("opening missing %s must fail with fs.ErrNotExist, got %v", missing, err)
		}
	}
}
