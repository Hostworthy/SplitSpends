// Package bootstrap is the composition root: it reads configuration and wires
// adapters into core services. It holds no business rules.
package bootstrap

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
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

	MCPEnabled bool

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

// hops reads TRUST_PROXY: a count of trusted proxies, or a boolean where
// true means one, as it did before counts were accepted.
func hops(v string) (int, error) {
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return n, nil
	}
	if b, err := strconv.ParseBool(v); err == nil {
		if b {
			return 1, nil
		}
		return 0, nil
	}
	return 0, fmt.Errorf("bootstrap.config.env.invalid.TRUST_PROXY: use the number of proxies in front of the server, or false")
}

// LoadConfig reads configuration from the environment.
func LoadConfig() (*Config, error) {
	c := &Config{
		AppName:      env("APP_NAME", "Split Spends"),
		Addr:         env("ADDR", ":8080"),
		BaseURL:      strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		DatabaseURL:  env("DATABASE_URL", ""),
		SignupMode:   auth.SignupMode(env("SIGNUP_MODE", string(auth.SignupInvite))),
		CORSOrigins:  list(env("CORS_ORIGINS", "")),
		DataDir:      env("DATA_DIR", "./data"),
		WebDir:       env("WEB_DIR", ""),
		LogLevel:     env("LOG_LEVEL", "info"),
		LogFormat:    env("LOG_FORMAT", "json"),
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
	var err error
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("bootstrap.config.env.required.not-found.DATABASE_URL")
	}
	secret := env("SECRET_KEY", "")
	if len(secret) < 32 {
		return nil, fmt.Errorf("bootstrap.config.env.invalid.SECRET_KEY: set at least 32 random characters, for example from `openssl rand -hex 32`")
	}
	if b, err := hex.DecodeString(secret); err == nil && len(b) >= 16 {
		c.Secret = b
	} else {
		c.Secret = []byte(secret)
	}
	switch c.SignupMode {
	case auth.SignupOpen, auth.SignupInvite, auth.SignupDomain, auth.SignupApproval, auth.SignupClosed:
	default:
		return nil, fmt.Errorf("bootstrap.config.env.invalid.SIGNUP_MODE: use open, invite, approval or closed")
	}
	c.AllowedDomains = list(env("SIGNUP_ALLOWED_DOMAINS", ""))
	if c.SignupMode == auth.SignupDomain && len(c.AllowedDomains) == 0 {
		return nil, fmt.Errorf("bootstrap.config.env.required.not-found.SIGNUP_ALLOWED_DOMAINS")
	}
	days, err := strconv.Atoi(env("SESSION_TTL_DAYS", "90"))
	if err != nil || days < 1 {
		return nil, fmt.Errorf("bootstrap.config.env.invalid.SESSION_TTL_DAYS")
	}
	c.SessionTTL = time.Duration(days) * 24 * time.Hour
	conns, err := strconv.Atoi(env("DATABASE_MAX_CONNS", "10"))
	if err != nil || conns < 1 {
		return nil, fmt.Errorf("bootstrap.config.env.invalid.DATABASE_MAX_CONNS")
	}
	c.DatabaseConns = int32(conns)
	if c.SMTPPort, err = strconv.Atoi(env("SMTP_PORT", "587")); err != nil {
		return nil, fmt.Errorf("bootstrap.config.env.invalid.SMTP_PORT")
	}
	if c.SMTPHost != "" && c.SMTPFrom == "" {
		c.SMTPFrom = c.SMTPUsername
	}
	if c.SMTPHost != "" && c.SMTPFrom == "" {
		return nil, fmt.Errorf("bootstrap.config.env.required.not-found.SMTP_FROM")
	}
	if c.ProxyHops, err = hops(env("TRUST_PROXY", "false")); err != nil {
		return nil, err
	}
	c.MCPEnabled, _ = strconv.ParseBool(env("MCP_ENABLED", "true"))
	switch strings.ToLower(env("GRAVATAR", "on")) {
	case "on", "true", "1":
		c.Gravatar = true
	case "off", "false", "0":
	default:
		return nil, fmt.Errorf("bootstrap.config.env.invalid.GRAVATAR: use on or off")
	}
	c.Serverless, _ = strconv.ParseBool(env("SERVERLESS", "false"))
	if err := c.loadPush(); err != nil {
		return nil, err
	}
	c.JobsSecret = env("JOBS_SECRET", "")
	if c.JobsSecret != "" && len(c.JobsSecret) < 32 {
		return nil, fmt.Errorf("bootstrap.config.env.invalid.JOBS_SECRET: use at least 32 random characters")
	}
	c.RatesURL = env("RATES_URL", exchange.DefaultURL)
	if strings.EqualFold(c.RatesURL, "none") {
		c.RatesURL = ""
	} else if !strings.HasPrefix(c.RatesURL, "https://") && !strings.HasPrefix(c.RatesURL, "http://") {
		return nil, fmt.Errorf("bootstrap.config.env.invalid.RATES_URL: use an http(s) URL or none")
	}
	c.BlobStore = strings.ToLower(env("BLOB_STORE", "local"))
	c.S3Bucket, c.S3Region, c.S3Endpoint = env("S3_BUCKET", ""), env("S3_REGION", env("AWS_REGION", "")), env("S3_ENDPOINT", "")
	c.S3AccessKey, c.S3SecretKey, c.S3Prefix = env("S3_ACCESS_KEY_ID", ""), env("S3_SECRET_ACCESS_KEY", ""), env("S3_PREFIX", "")
	switch c.BlobStore {
	case "local", "postgres":
	case "s3":
		if c.S3Bucket == "" {
			return nil, fmt.Errorf("bootstrap.config.env.required.not-found.S3_BUCKET")
		}
	default:
		return nil, fmt.Errorf("bootstrap.config.env.invalid.BLOB_STORE: use local, s3 or postgres")
	}
	switch c.AIProvider {
	case "", "none":
		c.AIProvider = ""
	case "anthropic":
		if c.AIAPIKey == "" {
			return nil, fmt.Errorf("bootstrap.config.env.required.not-found.AI_API_KEY")
		}
	case "openai":
		if c.AIBaseURL == "" || c.AIModel == "" {
			return nil, fmt.Errorf("bootstrap.config.env.required.not-found.AI_BASE_URL and AI_MODEL for AI_PROVIDER=openai")
		}
	default:
		return nil, fmt.Errorf("bootstrap.config.env.invalid.AI_PROVIDER: use anthropic, openai or none")
	}
	return c, nil
}
