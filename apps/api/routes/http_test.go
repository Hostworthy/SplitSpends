package routes

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

type sample struct {
	Name string `json:"name"`
}

func body(text string) (http.ResponseWriter, *http.Request) {
	return httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(text))
}

// refusal checks that err is a validation error whose message contains want.
func refusal(t *testing.T, err error, want string) {
	t.Helper()
	de, ok := errs.As(err)
	if !ok || de.Kind != errs.KindValidation || !strings.Contains(de.Message, want) {
		t.Fatalf("want a validation error saying %q, got %v", want, err)
	}
}

func TestDecodeTrailingData(t *testing.T) {
	for _, text := range []string{
		`{"name":"a"} {"name":"b"}`,
		`{"name":"a"} garbage`,
		`{"name":"a"}]`,
		`{"name":"a"} 1`,
	} {
		w, r := body(text)
		_, err := decode[sample](w, r)
		refusal(t, err, "only one JSON value")
	}
	w, r := body("{\"name\":\"a\"}\n  \n")
	if v, err := decode[sample](w, r); err != nil || v.Name != "a" {
		t.Fatalf("trailing white space is allowed: %+v, %v", v, err)
	}
}

func TestDecodeTooLarge(t *testing.T) {
	big := `{"name":"` + strings.Repeat("x", maxBody) + `"}`
	w, r := body(big)
	_, err := decode[sample](w, r)
	refusal(t, err, "too large")

	w, r = body(`{"name":"a"}` + strings.Repeat(" ", maxBody))
	_, err = decode[sample](w, r)
	refusal(t, err, "too large")

	w, r = body(`{"name":"` + strings.Repeat("x", 100) + `"}`)
	_, err = decodeLimit[sample](w, r, 64)
	refusal(t, err, "too large")
}

func TestDecodeUnknownFieldIgnored(t *testing.T) {
	w, r := body(`{"name":"a","addedLater":true}`)
	if v, err := decode[sample](w, r); err != nil || v.Name != "a" {
		t.Fatalf("unknown fields are ignored: %+v, %v", v, err)
	}
}

func TestDecodeEmpty(t *testing.T) {
	w, r := body(" ")
	_, err := decode[sample](w, r)
	refusal(t, err, "empty")

	w, r = body("")
	if v, err := decodeOptional[sample](w, r); err != nil || v != (sample{}) {
		t.Fatalf("an empty optional body is the zero value: %+v, %v", v, err)
	}
	// A body sent without a Content-Length is still read.
	r = httptest.NewRequest(http.MethodPost, "/", io.MultiReader(strings.NewReader(`{"name":"a"}`)))
	r.ContentLength = -1
	if v, err := decodeOptional[sample](httptest.NewRecorder(), r); err != nil || v.Name != "a" {
		t.Fatalf("want the body decoded, got %+v, %v", v, err)
	}
	w, r = body(`{"name":"a"} x`)
	_, err = decodeOptional[sample](w, r)
	refusal(t, err, "only one JSON value")
}
