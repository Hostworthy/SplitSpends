package admin

import (
	"reflect"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
)

func TestNormalize(t *testing.T) {
	s, err := Normalize(" Approval ", []string{"Ada@Example.com", "family.com", "@Family.com", "", "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Mode != auth.SignupApproval || !reflect.DeepEqual(s.Allowed, []string{"ada@example.com", "@family.com"}) {
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
}

func TestAllows(t *testing.T) {
	s := auth.Signup{Allowed: []string{"@family.com", "ben@example.com"}}
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
	p := auth.DefaultPolicy()
	p.Mode, p.AllowedDomains = auth.SignupDomain, []string{"Family.com", "@work.org"}
	got := p.Signup()
	if got.Mode != auth.SignupInvite || !reflect.DeepEqual(got.Allowed, []string{"@family.com", "@work.org"}) {
		t.Fatalf("got %+v", got)
	}
}
