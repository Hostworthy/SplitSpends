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
