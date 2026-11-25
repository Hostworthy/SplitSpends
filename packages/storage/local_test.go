package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStoresInsideRoot(t *testing.T) {
	ctx := context.Background()
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Put(ctx, "receipts/2026/03/a.png", strings.NewReader("image")); err != nil {
		t.Fatal(err)
	}
	body, err := l.Open(ctx, "receipts/2026/03/a.png")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(got) != "image" {
		t.Fatalf("want the stored bytes back, got %q, %v", got, err)
	}
	if err := l.Delete(ctx, "receipts/2026/03/a.png"); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(ctx, "receipts/2026/03/a.png"); err != nil {
		t.Fatalf("deleting a missing key succeeds, got %v", err)
	}
}

// TestLocalKeepsKeysInsideRoot checks that no key reaches a file outside
// the root, and that a key starting with a slash still lands inside it.
func TestLocalKeepsKeysInsideRoot(t *testing.T) {
	ctx := context.Background()
	parent := t.TempDir()
	root := filepath.Join(parent, "blobs")
	l, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", ".", "../secret", "a/../../secret", "..", "../blobs-other/x"} {
		if err := l.Put(ctx, key, strings.NewReader("x")); err == nil {
			t.Errorf("Put %q: want the key refused", key)
		}
		if body, err := l.Open(ctx, key); err == nil {
			_ = body.Close()
			t.Errorf("Open %q: want the key refused", key)
		}
		if err := l.Delete(ctx, key); err == nil {
			t.Errorf("Delete %q: want the key refused", key)
		}
	}
	if got, err := os.ReadFile(outside); err != nil || !bytes.Equal(got, []byte("secret")) {
		t.Fatalf("the file outside the root changed: %q, %v", got, err)
	}

	if err := l.Put(ctx, "/absolute", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "absolute")); err != nil {
		t.Fatalf("a key starting with a slash is stored under the root: %v", err)
	}
}
