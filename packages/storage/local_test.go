package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/storage/storagetest"
)

func TestLocalContract(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	storagetest.Run(t, l)
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
		if err := l.Put(ctx, key, []byte("x"), "image/png"); err == nil {
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

	if err := l.Put(ctx, "/absolute", []byte("x"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "absolute")); err != nil {
		t.Fatalf("a key starting with a slash is stored under the root: %v", err)
	}
}
