package bootstrap

import "testing"

func TestSecretsEndpoint(t *testing.T) {
	for _, tc := range []struct{ partition, region, want string }{
		{"aws", "eu-west-1", "https://secretsmanager.eu-west-1.amazonaws.com"},
		{"aws-us-gov", "us-gov-west-1", "https://secretsmanager.us-gov-west-1.amazonaws.com"},
		{"aws-cn", "cn-north-1", "https://secretsmanager.cn-north-1.amazonaws.com.cn"},
	} {
		if got := secretsEndpoint(tc.partition, tc.region); got != tc.want {
			t.Errorf("secretsEndpoint(%q, %q) = %q, want %q", tc.partition, tc.region, got, tc.want)
		}
	}
}
