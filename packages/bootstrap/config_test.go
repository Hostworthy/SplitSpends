package bootstrap_test

import (
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
)

// TestTrustProxy reads TRUST_PROXY as a hop count and keeps the boolean
// values deployments already use working.
func TestTrustProxy(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"", 0},
		{"false", 0},
		{"0", 0},
		{"true", 1},
		{"TRUE", 1},
		{"1", 1},
		{"2", 2},
	} {
		t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
		t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
		t.Setenv("TRUST_PROXY", tc.value)
		c, err := bootstrap.LoadConfig()
		if err != nil {
			t.Fatalf("%q: %v", tc.value, err)
		}
		if c.ProxyHops != tc.want {
			t.Errorf("%q: want %d hops, got %d", tc.value, tc.want, c.ProxyHops)
		}
	}
	for _, value := range []string{"-1", "yes", "two"} {
		t.Setenv("TRUST_PROXY", value)
		if _, err := bootstrap.LoadConfig(); err == nil || !strings.Contains(err.Error(), "TRUST_PROXY") {
			t.Errorf("%q: want a TRUST_PROXY error, got %v", value, err)
		}
	}
}

// TestDatabaseConns rejects pool sizes that do not fit the driver's int32
// instead of letting them wrap around.
func TestDatabaseConns(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
	t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
	for _, tc := range []struct {
		value string
		want  int32
	}{
		{"1", 1},
		{"10", 10},
		{"2147483647", 2147483647},
	} {
		t.Setenv("DATABASE_MAX_CONNS", tc.value)
		c, err := bootstrap.LoadConfig()
		if err != nil {
			t.Fatalf("%q: %v", tc.value, err)
		}
		if c.DatabaseConns != tc.want {
			t.Errorf("%q: want %d connections, got %d", tc.value, tc.want, c.DatabaseConns)
		}
	}
	for _, value := range []string{"0", "-1", "ten", "2147483648", "4294967306"} {
		t.Setenv("DATABASE_MAX_CONNS", value)
		if _, err := bootstrap.LoadConfig(); err == nil || !strings.Contains(err.Error(), "DATABASE_MAX_CONNS") {
			t.Errorf("%q: want a DATABASE_MAX_CONNS error, got %v", value, err)
		}
	}
}

// TestAdminEmail normalizes ADMIN_EMAIL like a sign-in address and rejects
// values that are not one.
func TestAdminEmail(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
	t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
	for _, tc := range []struct{ value, want string }{
		{"", ""},
		{" Owner@Example.com ", "owner@example.com"},
	} {
		t.Setenv("ADMIN_EMAIL", tc.value)
		c, err := bootstrap.LoadConfig()
		if err != nil {
			t.Fatalf("%q: %v", tc.value, err)
		}
		if c.AdminEmail != tc.want {
			t.Errorf("%q: want %q, got %q", tc.value, tc.want, c.AdminEmail)
		}
	}
	for _, value := range []string{"owner", "a@b.com,c@d.com"} {
		t.Setenv("ADMIN_EMAIL", value)
		if _, err := bootstrap.LoadConfig(); err == nil || !strings.Contains(err.Error(), "ADMIN_EMAIL") {
			t.Errorf("%q: want an ADMIN_EMAIL error, got %v", value, err)
		}
	}
}

// TestServerlessJobs requires JOBS_SECRET with SERVERLESS=true: without
// in-process jobs, only a scheduler calling /internal/jobs runs them.
func TestServerlessJobs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
	t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
	for _, tc := range []struct {
		serverless, secret string
		ok                 bool
	}{
		{"false", "", true},
		{"true", strings.Repeat("j", 32), true},
		{"true", "", false},
	} {
		t.Setenv("SERVERLESS", tc.serverless)
		t.Setenv("JOBS_SECRET", tc.secret)
		_, err := bootstrap.LoadConfig()
		if tc.ok && err != nil {
			t.Errorf("SERVERLESS=%s JOBS_SECRET=%q: %v", tc.serverless, tc.secret, err)
		}
		if !tc.ok && (err == nil || !strings.Contains(err.Error(), "JOBS_SECRET")) {
			t.Errorf("SERVERLESS=%s without JOBS_SECRET: want a JOBS_SECRET error, got %v", tc.serverless, err)
		}
	}
}

// TestAllowedDomains accepts domains with or without "@" and refuses
// entries that are not domains instead of starting with a list that
// matches nobody.
func TestAllowedDomains(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
	t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
	t.Setenv("SIGNUP_ALLOWED_DOMAINS", "Family.com, @work.org")
	if _, err := bootstrap.LoadConfig(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"not a domain", "family.com,example org", "ada@example.com", "@nodot"} {
		t.Setenv("SIGNUP_ALLOWED_DOMAINS", value)
		if _, err := bootstrap.LoadConfig(); err == nil || !strings.Contains(err.Error(), "invalid.SIGNUP_ALLOWED_DOMAINS") {
			t.Errorf("%q: want a SIGNUP_ALLOWED_DOMAINS error, got %v", value, err)
		}
	}
}
