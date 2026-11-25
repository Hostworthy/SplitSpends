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
		{"yes", 1},
		{"off", 0},
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
	for _, value := range []string{"-1", "maybe", "two"} {
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

func TestMCPAdminTools(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
	t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
	for _, tc := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"true", true}} {
		t.Setenv("MCP_ADMIN_TOOLS", tc.value)
		c, err := bootstrap.LoadConfig()
		if err != nil {
			t.Fatalf("MCP_ADMIN_TOOLS=%q: %v", tc.value, err)
		}
		if c.MCPAdminTools != tc.want {
			t.Errorf("MCP_ADMIN_TOOLS=%q: got %v, want %v", tc.value, c.MCPAdminTools, tc.want)
		}
	}
	t.Setenv("MCP_ADMIN_TOOLS", "yes please")
	if _, err := bootstrap.LoadConfig(); err == nil {
		t.Error("an invalid MCP_ADMIN_TOOLS must stop startup")
	}
}

// TestAIRateLimit reads the hourly AI budget per person and rejects values
// that would turn AI off or are not counts.
func TestAIRateLimit(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
	t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
	for _, tc := range []struct {
		value string
		want  int
	}{
		{"", 30},
		{"1", 1},
		{"200", 200},
	} {
		t.Setenv("AI_RATE_LIMIT", tc.value)
		c, err := bootstrap.LoadConfig()
		if err != nil {
			t.Fatalf("%q: %v", tc.value, err)
		}
		if c.AIRateLimit != tc.want {
			t.Errorf("%q: want %d requests, got %d", tc.value, tc.want, c.AIRateLimit)
		}
	}
	for _, value := range []string{"0", "-1", "ten"} {
		t.Setenv("AI_RATE_LIMIT", value)
		if _, err := bootstrap.LoadConfig(); err == nil || !strings.Contains(err.Error(), "AI_RATE_LIMIT") {
			t.Errorf("%q: want an AI_RATE_LIMIT error, got %v", value, err)
		}
	}
}

// TestLoadConfig covers the required and invalid branches of LoadConfig: a
// value it cannot read stops startup with the variable's key instead of
// quietly falling back to a default.
func TestLoadConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   map[string]string
		want  string
		check func(*bootstrap.Config) bool
	}{
		{name: "missing DATABASE_URL", env: map[string]string{"DATABASE_URL": ""}, want: "required.not-found.DATABASE_URL"},
		{name: "short SECRET_KEY", env: map[string]string{"SECRET_KEY": strings.Repeat("a", 31)}, want: "invalid.SECRET_KEY"},
		{name: "unknown SIGNUP_MODE", env: map[string]string{"SIGNUP_MODE": "bogus"}, want: "invalid.SIGNUP_MODE"},
		{name: "domain mode without domains", env: map[string]string{"SIGNUP_MODE": "domain"}, want: "required.not-found.SIGNUP_ALLOWED_DOMAINS"},
		{name: "unreadable SERVERLESS", env: map[string]string{"SERVERLESS": "maybe"}, want: "invalid.SERVERLESS"},
		{name: "SERVERLESS yes", env: map[string]string{"SERVERLESS": "yes", "JOBS_SECRET": strings.Repeat("j", 32)},
			check: func(c *bootstrap.Config) bool { return c.Serverless }},
		{name: "unreadable MCP_ENABLED", env: map[string]string{"MCP_ENABLED": "enabled"}, want: "invalid.MCP_ENABLED"},
		{name: "MCP_ENABLED off", env: map[string]string{"MCP_ENABLED": "OFF"},
			check: func(c *bootstrap.Config) bool { return !c.MCPEnabled }},
		{name: "unreadable GRAVATAR", env: map[string]string{"GRAVATAR": "sometimes"}, want: "invalid.GRAVATAR"},
		{name: "GRAVATAR no", env: map[string]string{"GRAVATAR": "no"},
			check: func(c *bootstrap.Config) bool { return !c.Gravatar }},
		{name: "unreadable PUSH_ALLOW_PRIVATE", env: map[string]string{"PUSH_ALLOW_PRIVATE": "lan"}, want: "invalid.PUSH_ALLOW_PRIVATE"},
		{name: "PUSH_ALLOW_PRIVATE on", env: map[string]string{"PUSH_ALLOW_PRIVATE": "on"},
			check: func(c *bootstrap.Config) bool { return c.PushAllowPrivate }},
		{name: "TRUST_PROXY yes", env: map[string]string{"TRUST_PROXY": "yes"},
			check: func(c *bootstrap.Config) bool { return c.ProxyHops == 1 }},
		{name: "unknown LOG_LEVEL", env: map[string]string{"LOG_LEVEL": "verbose"}, want: "invalid.LOG_LEVEL"},
		{name: "LOG_LEVEL in capitals", env: map[string]string{"LOG_LEVEL": "DEBUG"},
			check: func(c *bootstrap.Config) bool { return c.LogLevel == "debug" }},
		{name: "unknown LOG_FORMAT", env: map[string]string{"LOG_FORMAT": "xml"}, want: "invalid.LOG_FORMAT"},
		{name: "BASE_URL without scheme", env: map[string]string{"BASE_URL": "localhost:8080"}, want: "invalid.BASE_URL"},
		{name: "BASE_URL without host", env: map[string]string{"BASE_URL": "https://"}, want: "invalid.BASE_URL"},
		{name: "BASE_URL with trailing slash", env: map[string]string{"BASE_URL": "https://split.example.com/"},
			check: func(c *bootstrap.Config) bool { return c.BaseURL == "https://split.example.com" }},
		{name: "BASE_URL unset", check: func(c *bootstrap.Config) bool { return c.BaseURL == "http://localhost:8080" }},
		{name: "SMTP_PORT zero", env: map[string]string{"SMTP_PORT": "0"}, want: "invalid.SMTP_PORT"},
		{name: "SMTP_PORT too high", env: map[string]string{"SMTP_PORT": "65536"}, want: "invalid.SMTP_PORT"},
		{name: "short JOBS_SECRET", env: map[string]string{"JOBS_SECRET": strings.Repeat("j", 10)}, want: "invalid.JOBS_SECRET"},
		{name: "s3 without bucket", env: map[string]string{"BLOB_STORE": "s3"}, want: "required.not-found.S3_BUCKET"},
		{name: "openai without model", env: map[string]string{"AI_PROVIDER": "openai", "AI_BASE_URL": "https://ai.example.com/v1"}, want: "AI_MODEL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
			t.Setenv("SECRET_KEY", strings.Repeat("a", 32))
			for key, value := range tc.env {
				t.Setenv(key, value)
			}
			c, err := bootstrap.LoadConfig()
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("want an error naming %s, got %v", tc.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !tc.check(c) {
				t.Errorf("unexpected configuration: %+v", c)
			}
		})
	}
}
