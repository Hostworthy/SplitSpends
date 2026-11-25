// Package bootstrap is the composition root: it reads configuration and wires
// adapters into core services. It holds no business rules.
package bootstrap

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/exchange"
)

// Config is the full runtime configuration, read from the environment.
type Config struct {
	AppName        string
	Addr           string
	BaseURL        string
	DatabaseURL    string
	DatabaseConns  int32
	Secret         []byte
	SignupMode     auth.SignupMode
	AllowedDomains []string
	AdminEmail     string
	SessionTTL     time.Duration
	CORSOrigins    []string
	ProxyHops      int
	DataDir        string
	WebDir         string
	LogLevel       string
	LogFormat      string

	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPSecurity string

	AIProvider string
	AIAPIKey   string
	AIBaseURL  string
	AIModel    string
	AIEffort   string
	// AIRateLimit is how many AI requests one person may make per hour.
	AIRateLimit int

	MCPEnabled bool
	// MCPAdminTools offers the admin_ tools on /mcp.
	MCPAdminTools bool

	// Gravatar lets people show their Gravatar picture (opt-in per person).
	Gravatar bool

	// RatesURL is where exchange rates come from; empty turns them off.
	RatesURL string

	// VAPID identity for Web Push. When both keys are empty the server
	// generates a pair on first start and keeps it in the database.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
	// PushAllowPrivate lets push endpoints resolve to private addresses
	// (a UnifiedPush server on the local network).
	PushAllowPrivate bool

	// Serverless disables in-process background jobs and queued emails.
	Serverless bool
	JobsSecret string

	BlobStore   string
	S3Bucket    string
	S3Region    string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Prefix    string

	// baseURLDefault records that BASE_URL was not set, so links in emails
	// point at localhost.
	baseURLDefault bool
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func list(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// boolean reads the spellings of a switch people commonly write in
// environment files: true/false, 1/0, on/off and yes/no, in any case.
func boolean(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "on", "yes":
		return true, true
	case "off", "no":
		return false, true
	}
	b, err := strconv.ParseBool(v)
	return b, err == nil
}

// flag reads a boolean variable, refusing values it cannot read instead of
// treating them as false.
func flag(key string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	b, ok := boolean(strings.TrimSpace(v))
	if !ok {
		return false, fmt.Errorf("bootstrap.config.env.invalid.%s: use true or false (on/off, yes/no and 1/0 also work)", key)
	}
	return b, nil
}

// hops reads TRUST_PROXY: a count of trusted proxies, or a boolean where
// true means one, as it did before counts were accepted.
func hops(v string) (int, error) {
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return n, nil
	}
	if b, ok := boolean(v); ok {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, fmt.Errorf("bootstrap.config.env.invalid.TRUST_PROXY: use the number of proxies in front of the server, or false")
}

// baseURL checks BASE_URL is an absolute http(s) URL, since every link in
// emails and push messages starts with it.
func baseURL(v string) (string, error) {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("bootstrap.config.env.invalid.BASE_URL: use the public address with its scheme, for example https://split.example.com")
	}
	return strings.TrimRight(v, "/"), nil
}

// checkSecret refuses a SECRET_KEY that is short, a published example value
// or too uniform to be random. It keys the hashes of login codes, so a key
// someone can guess lets anyone who reads the stored hashes find the codes.
func checkSecret(secret string) error {
	if len(secret) < 32 {
		return fmt.Errorf("bootstrap.config.env.invalid.SECRET_KEY: set at least 32 random characters, for example from `openssl rand -hex 32`")
	}
	if strings.Contains(strings.ToLower(secret), "change-me") {
		return fmt.Errorf("bootstrap.config.env.invalid.SECRET_KEY: replace the example value with 32 random characters, for example from `openssl rand -hex 32`")
	}
	distinct := map[byte]bool{}
	for i := range len(secret) {
		distinct[secret[i]] = true
	}
	if len(distinct) < 10 {
		return fmt.Errorf("bootstrap.config.env.invalid.SECRET_KEY: use random characters, for example from `openssl rand -hex 32`; this value repeats too few of them")
	}
	return nil
}

// LoadConfig reads configuration from the environment, after filling it
// from the secret named by SECRETS_ARN when that is set.
func LoadConfig() (*Config, error) {
	if err := secrets(); err != nil {
		return nil, err
	}
	c := &Config{
		AppName:      env("APP_NAME", "Split Spends"),
		Addr:         env("ADDR", ":8080"),
		DatabaseURL:  env("DATABASE_URL", ""),
		SignupMode:   auth.SignupMode(env("SIGNUP_MODE", string(auth.SignupInvite))),
		CORSOrigins:  list(env("CORS_ORIGINS", "")),
		DataDir:      env("DATA_DIR", "./data"),
		WebDir:       env("WEB_DIR", ""),
		LogLevel:     strings.ToLower(env("LOG_LEVEL", "info")),
		LogFormat:    strings.ToLower(env("LOG_FORMAT", "json")),
		SMTPHost:     env("SMTP_HOST", ""),
		SMTPUsername: env("SMTP_USERNAME", ""),
		SMTPPassword: env("SMTP_PASSWORD", ""),
		SMTPFrom:     env("SMTP_FROM", ""),
		SMTPSecurity: env("SMTP_SECURITY", "starttls"),
		AIProvider:   strings.ToLower(env("AI_PROVIDER", "")),
		AIAPIKey:     env("AI_API_KEY", ""),
		AIBaseURL:    env("AI_BASE_URL", ""),
		AIModel:      env("AI_MODEL", ""),
		AIEffort:     env("AI_EFFORT", ""),
	}
	for _, load := range []func() error{c.loadDatabase, c.loadServer, c.loadAuth, c.loadSMTP, c.loadFlags, c.loadPush,
		c.loadJobs, c.loadRates, c.loadStorage, c.loadAI} {
		if err := load(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *Config) loadDatabase() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("bootstrap.config.env.required.not-found.DATABASE_URL")
	}
	conns, err := strconv.ParseInt(env("DATABASE_MAX_CONNS", "10"), 10, 32)
	if err != nil || conns < 1 {
		return fmt.Errorf("bootstrap.config.env.invalid.DATABASE_MAX_CONNS: use a whole number from 1 to 2147483647")
	}
	c.DatabaseConns = int32(conns)
	return nil
}

func (c *Config) loadServer() error {
	var err error
	c.baseURLDefault = env("BASE_URL", "") == ""
	if c.BaseURL, err = baseURL(env("BASE_URL", "http://localhost:8080")); err != nil {
		return err
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("bootstrap.config.env.invalid.LOG_LEVEL: use debug, info, warn or error")
	}
	switch c.LogFormat {
	case "json", "text":
	default:
		return fmt.Errorf("bootstrap.config.env.invalid.LOG_FORMAT: use json or text")
	}
	c.ProxyHops, err = hops(env("TRUST_PROXY", "false"))
	return err
}

func (c *Config) loadAuth() error {
	secret := env("SECRET_KEY", "")
	if err := checkSecret(secret); err != nil {
		return err
	}
	if b, err := hex.DecodeString(secret); err == nil && len(b) >= 16 {
		c.Secret = b
	} else {
		c.Secret = []byte(secret)
	}
	switch c.SignupMode {
	case auth.SignupOpen, auth.SignupInvite, auth.SignupDomain, auth.SignupApproval, auth.SignupClosed:
	default:
		return fmt.Errorf("bootstrap.config.env.invalid.SIGNUP_MODE: use open, invite, approval or closed (domain still works, as invite with SIGNUP_ALLOWED_DOMAINS)")
	}
	c.AllowedDomains = list(env("SIGNUP_ALLOWED_DOMAINS", ""))
	if len(c.AllowedDomains) > auth.MaxAllowed {
		return fmt.Errorf("bootstrap.config.env.invalid.SIGNUP_ALLOWED_DOMAINS: use at most %d domains", auth.MaxAllowed)
	}
	for _, d := range c.AllowedDomains {
		if _, ok := auth.Domain(d); !ok {
			return fmt.Errorf("bootstrap.config.env.invalid.SIGNUP_ALLOWED_DOMAINS: %q is not a domain like example.com", d)
		}
	}
	if c.SignupMode == auth.SignupDomain && len(c.AllowedDomains) == 0 {
		return fmt.Errorf("bootstrap.config.env.required.not-found.SIGNUP_ALLOWED_DOMAINS")
	}
	if admin := env("ADMIN_EMAIL", ""); admin != "" {
		var err error
		if c.AdminEmail, err = user.NormalizeEmail(admin); err != nil {
			return fmt.Errorf("bootstrap.config.env.invalid.ADMIN_EMAIL: use the one email address the first administrator signs in with")
		}
	}
	days, err := strconv.Atoi(env("SESSION_TTL_DAYS", "90"))
	if err != nil || days < 1 {
		return fmt.Errorf("bootstrap.config.env.invalid.SESSION_TTL_DAYS")
	}
	c.SessionTTL = time.Duration(days) * 24 * time.Hour
	return nil
}

func (c *Config) loadSMTP() error {
	var err error
	if c.SMTPPort, err = strconv.Atoi(env("SMTP_PORT", "587")); err != nil || c.SMTPPort < 1 || c.SMTPPort > 65535 {
		return fmt.Errorf("bootstrap.config.env.invalid.SMTP_PORT: use a port number from 1 to 65535")
	}
	if c.SMTPHost != "" && c.SMTPFrom == "" {
		c.SMTPFrom = c.SMTPUsername
	}
	if c.SMTPHost != "" && c.SMTPFrom == "" {
		return fmt.Errorf("bootstrap.config.env.required.not-found.SMTP_FROM")
	}
	return nil
}

func (c *Config) loadFlags() error {
	var err error
	if c.MCPEnabled, err = flag("MCP_ENABLED", true); err != nil {
		return err
	}
	if c.MCPAdminTools, err = flag("MCP_ADMIN_TOOLS", false); err != nil {
		return err
	}
	if c.Gravatar, err = flag("GRAVATAR", true); err != nil {
		return err
	}
	c.Serverless, err = flag("SERVERLESS", false)
	return err
}

func (c *Config) loadJobs() error {
	c.JobsSecret = env("JOBS_SECRET", "")
	if c.JobsSecret != "" && len(c.JobsSecret) < 32 {
		return fmt.Errorf("bootstrap.config.env.invalid.JOBS_SECRET: use at least 32 random characters")
	}
	// Without in-process jobs, a scheduler calling /internal/jobs is the only
	// thing that books recurring expenses and removes expired sign-ins.
	if c.Serverless && c.JobsSecret == "" {
		return fmt.Errorf("bootstrap.config.env.required.not-found.JOBS_SECRET: SERVERLESS=true needs a scheduler that calls POST /internal/jobs with it")
	}
	return nil
}

func (c *Config) loadRates() error {
	c.RatesURL = env("RATES_URL", exchange.DefaultURL)
	if strings.EqualFold(c.RatesURL, "none") {
		c.RatesURL = ""
	} else if !strings.HasPrefix(c.RatesURL, "https://") && !strings.HasPrefix(c.RatesURL, "http://") {
		return fmt.Errorf("bootstrap.config.env.invalid.RATES_URL: use an http(s) URL or none")
	}
	return nil
}

func (c *Config) loadStorage() error {
	c.BlobStore = strings.ToLower(env("BLOB_STORE", "local"))
	c.S3Bucket, c.S3Region, c.S3Endpoint = env("S3_BUCKET", ""), env("S3_REGION", env("AWS_REGION", "")), env("S3_ENDPOINT", "")
	c.S3AccessKey, c.S3SecretKey, c.S3Prefix = env("S3_ACCESS_KEY_ID", ""), env("S3_SECRET_ACCESS_KEY", ""), env("S3_PREFIX", "")
	switch c.BlobStore {
	case "local", "postgres":
	case "s3":
		if c.S3Bucket == "" {
			return fmt.Errorf("bootstrap.config.env.required.not-found.S3_BUCKET")
		}
	default:
		return fmt.Errorf("bootstrap.config.env.invalid.BLOB_STORE: use local, s3 or postgres")
	}
	return nil
}

func (c *Config) loadAI() error {
	switch c.AIProvider {
	case "", "none":
		c.AIProvider = ""
	case "anthropic":
		if c.AIAPIKey == "" {
			return fmt.Errorf("bootstrap.config.env.required.not-found.AI_API_KEY")
		}
	case "openai":
		if c.AIBaseURL == "" || c.AIModel == "" {
			return fmt.Errorf("bootstrap.config.env.required.not-found.AI_BASE_URL and AI_MODEL for AI_PROVIDER=openai")
		}
	default:
		return fmt.Errorf("bootstrap.config.env.invalid.AI_PROVIDER: use anthropic, openai or none")
	}
	var err error
	if c.AIRateLimit, err = strconv.Atoi(env("AI_RATE_LIMIT", "30")); err != nil || c.AIRateLimit < 1 {
		return fmt.Errorf("bootstrap.config.env.invalid.AI_RATE_LIMIT: use a whole number of requests per person per hour, at least 1")
	}
	return nil
}
