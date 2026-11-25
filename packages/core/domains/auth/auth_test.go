package auth

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	s, err := Normalize(" Approval ", []string{"Ada@Example.com", "family.com", "@Family.com", "", "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != SignupApproval || !reflect.DeepEqual(s.Allowed, []string{"ada@example.com", "@family.com"}) {
		t.Fatalf("got %+v", s)
	}
	for _, bad := range []string{"not an email", "@nodot", "@bad_domain.com"} {
		if _, err := Normalize("invite", []string{bad}); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	if _, err := Normalize("domain", nil); err == nil {
		t.Error("the old domain mode is not offered to administrators")
	}
	if _, err := Normalize("invite", make([]string, MaxAllowed+1)); err == nil || !strings.Contains(err.Error(), "500 entries") {
		t.Errorf("an overlong list must be refused with its limit: %v", err)
	}
}

func TestDomain(t *testing.T) {
	cases := map[string]string{
		"Family.com":      "@family.com",
		" @work.org ":     "@work.org",
		"sub.example.com": "@sub.example.com",
		"not a domain":    "",
		"example org":     "",
		"@nodot":          "",
		"ada@example.com": "",
		".example.com":    "",
	}
	for raw, want := range cases {
		got, ok := Domain(raw)
		if got != want || ok != (want != "") {
			t.Errorf("Domain(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
}

func TestAllows(t *testing.T) {
	s := Signup{Allowed: []string{"@family.com", "ben@example.com"}}
	cases := map[string]bool{
		"kim@family.com":     true,
		"KIM@Family.com":     true,
		"kim@sub.family.com": false,
		"ben@example.com":    true,
		"ada@example.com":    false,
		"family.com":         false,
	}
	for email, want := range cases {
		if got := s.Allows(email); got != want {
			t.Errorf("Allows(%q) = %v", email, got)
		}
	}
}

func TestPolicySignupMapsDomainMode(t *testing.T) {
	p := DefaultPolicy()
	p.Mode, p.AllowedDomains = SignupDomain, []string{"Family.com", "@work.org", "family.com"}
	got := p.Signup()
	if got.Mode != SignupInvite || !reflect.DeepEqual(got.Allowed, []string{"@family.com", "@work.org"}) {
		t.Fatalf("got %+v", got)
	}
}
