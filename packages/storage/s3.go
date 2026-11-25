package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config selects an S3-compatible bucket: AWS S3, Cloudflare R2, MinIO,
// Google Cloud Storage (interoperability keys) and others.
type S3Config struct {
	Bucket    string
	Region    string
	Endpoint  string // [https://|http://]host[:port]; defaults to AWS for Region
	AccessKey string // optional; falls back to AWS_* env and instance roles
	SecretKey string
	Prefix    string
}

// S3 stores blobs in a bucket. Objects stay private; the API streams them
// to authorized callers.
type S3 struct {
	client *minio.Client
	bucket string
	prefix string
}

// NewS3 builds an S3 store.
func NewS3(cfg S3Config) (*S3, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
		if cfg.Region != "" && cfg.Region != "us-east-1" {
			endpoint = "s3." + cfg.Region + ".amazonaws.com"
		}
	}
	// Only an explicit http:// turns TLS off, for a MinIO on a local network.
	secure := true
	if scheme, host, ok := strings.Cut(endpoint, "://"); ok {
		secure = !strings.EqualFold(scheme, "http")
		endpoint = host
	}
	var creds *credentials.Credentials
	if cfg.AccessKey != "" {
		creds = credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	} else {
		// Lambda, ECS and EC2 roles expose credentials through AWS_* env vars
		// or the instance metadata endpoint.
		creds = credentials.NewChainCredentials([]credentials.Provider{&credentials.EnvAWS{}, &credentials.IAM{}})
	}
	client, err := minio.New(endpoint, &minio.Options{Creds: creds, Secure: secure, Region: cfg.Region})
	if err != nil {
		return nil, fmt.Errorf("storage.s3.config.invalid: %w", err)
	}
	prefix := strings.Trim(cfg.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &S3{client: client, bucket: cfg.Bucket, prefix: prefix}, nil
}

// Secure reports whether the store talks to its endpoint over TLS.
func (s *S3) Secure() bool { return s.client.EndpointURL().Scheme == "https" }

func (s *S3) Put(ctx context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	_, err = s.client.PutObject(ctx, s.bucket, s.prefix+key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return fmt.Errorf("storage.s3.put.failed: %w", err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, s.prefix+key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("storage.s3.get.failed: %w", err)
	}
	// GetObject only connects on the first read; asking for the object's
	// details now lets a missing object fail here, before a caller has
	// started its response.
	if _, err := obj.Stat(); err != nil {
		_ = obj.Close()
		if minio.ToErrorResponse(err).Code == minio.NoSuchKey {
			return nil, &fs.PathError{Op: "open", Path: key, Err: fs.ErrNotExist}
		}
		return nil, fmt.Errorf("storage.s3.get.failed: %w", err)
	}
	return obj, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, s.prefix+key, minio.RemoveObjectOptions{})
}
