package storage

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/storage/storagetest"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// bucket is the part of the S3 API the store uses, kept in memory: objects
// are put, read, checked and removed by path.
type bucket struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (b *bucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		b.objects[r.URL.Path] = data
		w.Header().Set("ETag", `"etag"`)
	case http.MethodGet, http.MethodHead:
		data, ok := b.objects[r.URL.Path]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
			}
			return
		}
		w.Header().Set("ETag", `"etag"`)
		w.Header().Set("Last-Modified", "Mon, 02 Mar 2026 10:00:00 GMT")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	case http.MethodDelete:
		delete(b.objects, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestS3Contract(t *testing.T) {
	srv := httptest.NewTLSServer(&bucket{objects: map[string][]byte{}})
	t.Cleanup(srv.Close)
	client, err := minio.New(strings.TrimPrefix(srv.URL, "https://"), &minio.Options{
		Creds: credentials.NewStaticV4("key", "secret", ""), Secure: true, Region: "us-east-1",
		Transport: srv.Client().Transport, BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	storagetest.Run(t, &S3{client: client, bucket: "receipts", prefix: "test/"})
}
