package bootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/signer"
)

// secrets copies the JSON object stored in the AWS Secrets Manager secret
// named by SECRETS_ARN into the environment, so hosts such as Lambda can keep
// DATABASE_URL, SECRET_KEY and the other secrets out of the function's
// configuration while the server still reads them under their usual names.
// Variables that are already set win.
func secrets() error {
	arn := env("SECRETS_ARN", "")
	if arn == "" {
		return nil
	}
	parts := strings.SplitN(arn, ":", 7)
	if len(parts) != 7 || parts[0] != "arn" || parts[2] != "secretsmanager" || parts[3] == "" || parts[5] != "secret" {
		return fmt.Errorf("bootstrap.config.env.invalid: SECRETS_ARN: use the ARN of a Secrets Manager secret, arn:aws:secretsmanager:<region>:<account>:secret:<name>")
	}
	region := parts[3]
	value, err := secretValue(arn, region, env("AWS_ENDPOINT_URL_SECRETS_MANAGER", secretsEndpoint(parts[1], region)))
	if err != nil {
		return fmt.Errorf("bootstrap.config.secrets.failed: could not read SECRETS_ARN from Secrets Manager: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(value), &values); err != nil || values == nil {
		return fmt.Errorf("bootstrap.config.secrets.invalid: the SECRETS_ARN secret must be a JSON object of strings, such as {\"SECRET_KEY\": \"...\"}")
	}
	for key, v := range values {
		if env(key, "") == "" {
			if err := os.Setenv(key, v); err != nil {
				return fmt.Errorf("bootstrap.config.secrets.invalid: the SECRETS_ARN secret has a key that is not a valid variable name: %w", err)
			}
		}
	}
	return nil
}

// secretsEndpoint is the regional Secrets Manager endpoint for the partition
// named in the ARN. The China regions have their own domain; other isolated
// partitions set AWS_ENDPOINT_URL_SECRETS_MANAGER.
func secretsEndpoint(partition, region string) string {
	domain := "amazonaws.com"
	if partition == "aws-cn" {
		domain = "amazonaws.com.cn"
	}
	return "https://secretsmanager." + region + "." + domain
}

// secretValue calls GetSecretValue with the credentials Lambda, ECS and EC2
// provide through the AWS_* variables or the instance metadata endpoint.
func secretValue(arn, region, endpoint string) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	chain := credentials.NewChainCredentials([]credentials.Provider{&credentials.EnvAWS{}, &credentials.IAM{}})
	creds, err := chain.GetWithContext(&credentials.CredContext{Client: client})
	if err != nil {
		return "", fmt.Errorf("no AWS credentials: %w", err)
	}
	body, err := json.Marshal(map[string]string{"SecretId": arn})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "secretsmanager.GetSecretValue")
	// The signer takes the payload hash from this header; without it the
	// request would be signed as an unsigned payload, which only S3 accepts.
	req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(sum[:]))
	req = signer.SignV4WithServiceType(*req, creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken, region, "secretsmanager")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out struct {
		SecretString string
		Type         string `json:"__type"`
		Message      string
		LowerMessage string `json:"message"`
	}
	_ = json.Unmarshal(data, &out)
	if resp.StatusCode != http.StatusOK {
		msg := out.Message
		if msg == "" {
			msg = out.LowerMessage
		}
		return "", fmt.Errorf("%s %s: %s", resp.Status, out.Type, msg)
	}
	return out.SecretString, nil
}
