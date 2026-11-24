package user

import (
	"testing"
	"time"
)

func TestPicture(t *testing.T) {
	u := &User{Email: "  Ada@Example.com "}
	if got := u.Picture(160); got != "" {
		t.Fatalf("no picture before opting in, got %q", got)
	}
	u.Gravatar = true
	// SHA-256 of "ada@example.com".
	want := "https://gravatar.com/avatar/b5fc85e55755f9e0d030a10ab4429b6b2944855f9a0d60077fe832becbc41d72?s=160&d=404"
	if got := u.Picture(160); got != want {
		t.Fatalf("picture = %q", got)
	}
	if u.Picture(160) != (&User{Email: "ada@example.com", Gravatar: true}).Picture(160) {
		t.Fatal("the hash must ignore case and spaces")
	}

	placeholder := &User{Name: "Sam"}
	on := true
	if err := placeholder.Apply(Patch{Gravatar: &on}, time.Now()); err == nil {
		t.Fatal("people without email cannot use Gravatar")
	}
}
