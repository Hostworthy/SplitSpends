package bootstrap_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
)

const secretARN = "arn:aws:secretsmanager:eu-west-1:123456789012:secret:splitspends-app-AbCdEf"

// secretsManager serves GetSecretValue for secretARN with the given status
// and body, and fails the test on a request that is not signed for it.
func secretsManager(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		sum := sha256.Sum256(payload)
		var in struct{ SecretId string }
		_ = json.Unmarshal(payload, &in)
		switch {
		case r.Method != http.MethodPost || r.Header.Get("X-Amz-Target") != "secretsmanager.GetSecretValue":
			t.Errorf("want a GetSecretValue call, got %s %q", r.Method, r.Header.Get("X-Amz-Target"))
		case in.SecretId != secretARN:
			t.Errorf("want SecretId %q, got %q", secretARN, in.SecretId)
		case !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") ||
			!strings.Contains(r.Header.Get("Authorization"), "/eu-west-1/secretsmanager/aws4_request"):
			t.Errorf("want a SigV4 signature for secretsmanager in eu-west-1, got %q", r.Header.Get("Authorization"))
		case r.Header.Get("X-Amz-Security-Token") != "session-token":
			t.Errorf("want the session token, got %q", r.Header.Get("X-Amz-Security-Token"))
		case r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]):
			t.Errorf("want the payload hash signed, got %q", r.Header.Get("X-Amz-Content-Sha256"))
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AWS_ENDPOINT_URL_SECRETS_MANAGER", srv.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_SESSION_TOKEN", "session-token")
	t.Setenv("SECRETS_ARN", secretARN)
}

func secretString(t *testing.T, values map[string]string) string {
	t.Helper()
	inner, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := json.Marshal(map[string]string{"ARN": secretARN, "SecretString": string(inner)})
	if err != nil {
		t.Fatal(err)
	}
	return string(outer)
}

// TestSecrets reads the secret named by SECRETS_ARN into the usual
// variables, leaving the ones already set alone.
func TestSecrets(t *testing.T) {
	secretsManager(t, http.StatusOK, secretString(t, map[string]string{
		"DATABASE_URL":  "postgres://app:pw@db.example.com/splitspends?sslmode=require",
		"SECRET_KEY":    "secret from the secrets manager, not hex",
		"JOBS_SECRET":   strings.Repeat("c", 48),
		"SMTP_PASSWORD": "from-secret",
		"AI_API_KEY":    "",
	}))
	// Registered so the test restores them; empty counts as unset.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SECRET_KEY", "")
	t.Setenv("JOBS_SECRET", "")
	t.Setenv("AI_API_KEY", "")
	t.Setenv("SMTP_PASSWORD", "explicit")

	c, err := bootstrap.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.DatabaseURL != "postgres://app:pw@db.example.com/splitspends?sslmode=require" {
		t.Errorf("want DATABASE_URL from the secret, got %q", c.DatabaseURL)
	}
	if string(c.Secret) != "secret from the secrets manager, not hex" {
		t.Errorf("want SECRET_KEY from the secret, got %q", c.Secret)
	}
	if c.JobsSecret != strings.Repeat("c", 48) {
		t.Errorf("want JOBS_SECRET from the secret, got %q", c.JobsSecret)
	}
	if c.SMTPPassword != "explicit" {
		t.Errorf("want SMTP_PASSWORD already set to win, got %q", c.SMTPPassword)
	}
	if c.AIAPIKey != "" {
		t.Errorf("want an empty AI_API_KEY, got %q", c.AIAPIKey)
	}
}

// TestSecretsFailures stops startup with a message naming SECRETS_ARN when
// the secret cannot be used.
func TestSecretsFailures(t *testing.T) {
	for _, tc := range []struct {
		name, arn, body, want string
		status                int
	}{
		{"not an ARN", "splitspends-app", "", "bootstrap.config.env.invalid: SECRETS_ARN", http.StatusOK},
		{"not Secrets Manager", "arn:aws:ssm:eu-west-1:123456789012:parameter/app", "", "bootstrap.config.env.invalid: SECRETS_ARN", http.StatusOK},
		{"denied", secretARN, `{"__type":"AccessDeniedException","Message":"not authorized"}`, "AccessDeniedException: not authorized", http.StatusBadRequest},
		{"not an object", secretARN, `{"SecretString":"plain text"}`, "a JSON object of strings", http.StatusOK},
		{"null", secretARN, `{"SecretString":"null"}`, "a JSON object of strings", http.StatusOK},
		{"not strings", secretARN, `{"SecretString":"{\"SMTP_PORT\":587}"}`, "a JSON object of strings", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secretsManager(t, tc.status, tc.body)
			t.Setenv("SECRETS_ARN", tc.arn)
			t.Setenv("DATABASE_URL", "postgres://localhost/splitspends")
			t.Setenv("SECRET_KEY", secretKey)
			_, err := bootstrap.LoadConfig()
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "SECRETS_ARN") {
				t.Errorf("want an error with %q naming SECRETS_ARN, got %v", tc.want, err)
			}
		})
	}
}
