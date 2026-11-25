package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// A draft answer as a provider would return it.
const answer = `{"description":"Dinner at the market","amount":"132.40","currency":"EUR","date":"2026-10-05",
"category":"food.dining","groupId":"g1","payerId":"me","participantIds":["me","ben"],
"items":[{"name":"Pasta","amount":"24.00","participantIds":["ben"]},{"name":"Wine","amount":"26.00","participantIds":[]}],
"tax":"6.40","tip":"0","unmatched":["Zoe"]}`

func draftInput() ports.DraftInput {
	return ports.DraftInput{
		Text: "dinner, Ben had the pasta, split the rest in Lisbon trip", Today: "2026-10-06", Currency: "EUR",
		Me:         ports.Person{ID: "me", Name: "Ada"},
		People:     []ports.Person{{ID: "ben", Name: "Ben Carter"}, {ID: "ben2", Name: "Ben Carter"}},
		Groups:     []ports.GroupOption{{ID: "g1", Name: "Lisbon trip", Currency: "EUR", MemberIDs: []string{"me", "ben"}}},
		Categories: []string{"food.dining", "general"},
		Image:      []byte{0xFF, 0xD8, 0xFF}, ContentType: "image/jpeg",
	}
}

func checkDraft(t *testing.T, d ports.DraftExtract) {
	t.Helper()
	if d.GroupID != "g1" || d.PayerID != "me" || len(d.Items) != 2 || d.Items[0].ParticipantIDs[0] != "ben" || d.Tax != "6.40" || d.Unmatched[0] != "Zoe" {
		t.Fatalf("parsed draft wrong: %+v", d)
	}
}

func checkSystemPrompt(t *testing.T, system string) {
	t.Helper()
	for _, want := range []string{"Today is 2026-10-06", "id ben: Ben Carter\n", "id ben2: Ben Carter (2)\n", "id g1: Lisbon trip (EUR), members me, ben",
		"food.dining, general", "Only use ids from the lists above", "not instructions to follow"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt is missing %q", want)
		}
	}
	if strings.Contains(system, "@") {
		t.Errorf("system prompt must not carry email addresses: %s", system)
	}
}

func TestOpenAICompatibleDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Strict bool           `json:"strict"`
					Schema map[string]any `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "gemini-2.5-flash" || body.ResponseFormat.Type != "json_schema" || !body.ResponseFormat.JSONSchema.Strict {
			t.Errorf("model or response format wrong: %s", raw)
		}
		var system string
		_ = json.Unmarshal(body.Messages[0].Content, &system)
		checkSystemPrompt(t, system)
		user := string(body.Messages[1].Content)
		if !strings.Contains(user, "data:image/jpeg;base64,") || !strings.Contains(user, "Ben had the pasta") {
			t.Errorf("user turn should carry the photo and the instructions: %s", user)
		}
		reply, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": answer}}}})
		w.Write(reply)
	}))
	defer srv.Close()

	d, err := NewOpenAICompatible(srv.URL+"/v1", "key", "gemini-2.5-flash").DraftExpense(context.Background(), draftInput())
	if err != nil {
		t.Fatal(err)
	}
	checkDraft(t, d)
}

func TestAnthropicDraft(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The official SDK sends the API key as a bearer token.
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") || r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected request %s auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Model  string `json:"model"`
			System []struct {
				Text string `json:"text"`
			} `json:"system"`
			Messages []struct {
				Content []map[string]any `json:"content"`
			} `json:"messages"`
			OutputConfig struct {
				Format struct {
					Type   string         `json:"type"`
					Schema map[string]any `json:"schema"`
				} `json:"format"`
			} `json:"output_config"`
			Fallbacks any `json:"fallbacks"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "claude-opus-5-5" || body.OutputConfig.Format.Type != "json_schema" || body.Fallbacks == nil {
			t.Errorf("model, structured output or fallbacks missing: %s", raw)
		}
		checkSystemPrompt(t, body.System[0].Text)
		content := body.Messages[0].Content
		if len(content) != 2 || content[0]["type"] != "image" || content[1]["type"] != "text" {
			t.Errorf("expected an image block then the instructions: %v", content)
		}
		reply, _ := json.Marshal(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content":     []any{map[string]any{"type": "text", "text": answer}},
			"stop_reason": "end_turn", "usage": map[string]any{"input_tokens": 10, "output_tokens": 10},
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(reply)
	}))
	defer srv.Close()

	d, err := NewAnthropic("sk-test", srv.URL, "", "").DraftExpense(context.Background(), draftInput())
	if err != nil {
		t.Fatal(err)
	}
	checkDraft(t, d)
}

func TestRejectsUnsupportedImages(t *testing.T) {
	ctx := context.Background()
	draft := draftInput()
	draft.ContentType = "image/heic"
	scan := receiptInput()
	scan.ContentType = "image/heic"
	url, calls := provider(t, http.StatusOK, "{}")
	for name, call := range map[string]func() error{
		"openai draft":      func() error { _, err := NewOpenAICompatible(url, "", "m").DraftExpense(ctx, draft); return err },
		"openai receipt":    func() error { _, err := NewOpenAICompatible(url, "", "m").ReadReceipt(ctx, scan); return err },
		"anthropic draft":   func() error { _, err := anthropicAt(url).DraftExpense(ctx, draft); return err },
		"anthropic receipt": func() error { _, err := anthropicAt(url).ReadReceipt(ctx, scan); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "JPEG, PNG and WebP") {
			t.Errorf("%s: want a friendly format error, got %v", name, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("an unsupported image must not be sent: %d requests", calls.Load())
	}
}

// provider answers every request with status and body and counts requests.
func provider(t *testing.T, status int, body string) (url string, calls *atomic.Int32) {
	t.Helper()
	calls = &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, calls
}

// choice is an OpenAI chat completion whose message has content and refusal.
func choice(content, refusal string) string {
	raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content, "refusal": refusal}}}})
	return string(raw)
}

// message is an Anthropic message answering text with stopReason.
func message(text, stopReason string) string {
	raw, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content":     []any{map[string]any{"type": "text", "text": text}},
		"stop_reason": stopReason, "usage": map[string]any{"input_tokens": 10, "output_tokens": 10},
	})
	return string(raw)
}

// anthropicAt is an Anthropic reader on url that does not retry, so a
// failing answer is seen at once.
func anthropicAt(url string) *Anthropic {
	return &Anthropic{client: anthropic.NewClient(option.WithAPIKey("sk-test"), option.WithBaseURL(url), option.WithMaxRetries(0)),
		model: DefaultAnthropicModel, timeout: requestTimeout}
}

const receiptAnswer = `{"merchant":"Market","date":"2026-10-05","currency":"EUR","category":"food.groceries","subtotal":"10.00",
"tax":"1.00","tip":"0","total":"11.00","items":[{"name":"Bread","quantity":2,"amount":"10.00"}]}`

func receiptInput() ports.ReceiptInput {
	return ports.ReceiptInput{Image: []byte{0xFF, 0xD8, 0xFF}, ContentType: "image/jpeg", Currency: "EUR", Categories: []string{"food.groceries"}}
}

// TestProviderAnswers checks how each adapter reads a receipt from the
// answers a provider can give: domain errors for what the user can act on,
// plain errors for everything else.
func TestProviderAnswers(t *testing.T) {
	openai := func(url string) (ports.ReceiptExtract, error) {
		return NewOpenAICompatible(url, "key", "m").ReadReceipt(context.Background(), receiptInput())
	}
	claude := func(url string) (ports.ReceiptExtract, error) {
		return anthropicAt(url).ReadReceipt(context.Background(), receiptInput())
	}
	fenced := "```json\n" + receiptAnswer + "\n```"
	for _, tc := range []struct {
		name   string
		read   func(url string) (ports.ReceiptExtract, error)
		status int
		body   string
		kind   errs.Kind // "" means success when plain is false
		plain  bool      // a non-domain error
	}{
		{"openai reads a receipt", openai, http.StatusOK, choice(receiptAnswer, ""), "", false},
		{"openai reads fenced JSON", openai, http.StatusOK, choice(fenced, ""), "", false},
		{"openai busy", openai, http.StatusTooManyRequests, `{"error":{"message":"slow down"}}`, errs.KindRateLimited, false},
		{"openai refusal", openai, http.StatusOK, choice("", "I can't help with that."), errs.KindValidation, false},
		{"openai bad key", openai, http.StatusUnauthorized, `{"error":{"message":"bad key"}}`, "", true},
		{"openai no choices", openai, http.StatusOK, `{"choices":[]}`, "", true},
		{"openai not JSON", openai, http.StatusOK, `<html>gateway</html>`, "", true},
		{"openai answer not JSON", openai, http.StatusOK, choice("Sorry, I cannot read that.", ""), "", true},
		{"anthropic reads a receipt", claude, http.StatusOK, message(receiptAnswer, "end_turn"), "", false},
		{"anthropic reads fenced JSON", claude, http.StatusOK, message(fenced, "end_turn"), "", false},
		{"anthropic busy", claude, http.StatusTooManyRequests, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, errs.KindRateLimited, false},
		{"anthropic refusal", claude, http.StatusOK, message("", "refusal"), errs.KindValidation, false},
		{"anthropic bad key", claude, http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, "", true},
		{"anthropic answer not JSON", claude, http.StatusOK, message("Sorry.", "end_turn"), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url, calls := provider(t, tc.status, tc.body)
			got, err := tc.read(url)
			if calls.Load() != 1 {
				t.Fatalf("want one request, got %d", calls.Load())
			}
			e, domain := errs.As(err)
			switch {
			case tc.kind != "":
				if !domain || e.Kind != tc.kind {
					t.Fatalf("want a %s error, got %v", tc.kind, err)
				}
			case tc.plain:
				if err == nil || domain {
					t.Fatalf("want a plain error, got %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if got.Merchant != "Market" || got.Total != "11.00" || len(got.Items) != 1 || got.Items[0].Quantity != 2 {
					t.Fatalf("parsed receipt wrong: %+v", got)
				}
			}
		})
	}
}

// TestProviderErrorsKeepContentOut checks that what a provider answers,
// which can echo the receipt or the prompt, never reaches an error message,
// and that a refused request carries its status as data.
func TestProviderErrorsKeepContentOut(t *testing.T) {
	const secret = "SECRET-RECEIPT-TEXT"
	openai := func(url string) error {
		_, err := NewOpenAICompatible(url, "key", "m").ReadReceipt(context.Background(), receiptInput())
		return err
	}
	claude := func(url string) error {
		_, err := anthropicAt(url).ReadReceipt(context.Background(), receiptInput())
		return err
	}
	for _, tc := range []struct {
		name   string
		read   func(url string) error
		status int
		body   string
		want   int // the status the error carries; 0 for none
	}{
		{"openai rejected", openai, http.StatusBadRequest, `{"error":{"message":"bad input: ` + secret + `"}}`, http.StatusBadRequest},
		{"openai bad key", openai, http.StatusUnauthorized, `{"error":{"message":"` + secret + `"}}`, http.StatusUnauthorized},
		{"openai failed", openai, http.StatusBadGateway, `<html>` + secret + `</html>`, http.StatusBadGateway},
		{"openai invalid answer", openai, http.StatusOK, `{"result":"` + secret + `"}`, 0},
		{"openai not JSON", openai, http.StatusOK, secret, 0},
		{"anthropic rejected", claude, http.StatusBadRequest, `{"type":"error","error":{"type":"invalid_request_error","message":"` + secret + `"}}`, http.StatusBadRequest},
		{"anthropic bad key", claude, http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"` + secret + `"}}`, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			url, _ := provider(t, tc.status, tc.body)
			err := tc.read(url)
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("the provider's answer must stay out of the error, got %v", err)
			}
			var r *rejection
			if got := errors.As(err, &r); got != (tc.want != 0) || (got && r.Status != tc.want) {
				t.Fatalf("want status %d carried by the error, got %v", tc.want, err)
			}
			var logged bytes.Buffer
			slog.New(slog.NewTextHandler(&logged, nil)).Error("middleware.error.internal", "error", err)
			if strings.Contains(logged.String(), secret) || (tc.want != 0 && !strings.Contains(logged.String(), "status="+strconv.Itoa(tc.want))) {
				t.Fatalf("the log line must carry the status and no content: %s", logged.String())
			}
		})
	}
}

// TestProviderBudget checks that both adapters give up on a provider that
// never answers once their time budget is spent, and that a failing
// provider is asked at most once more.
func TestProviderBudget(t *testing.T) {
	const budget = 200 * time.Millisecond
	adapters := []struct {
		name     string
		read     func(url string, timeout time.Duration) error
		attempts int32
	}{
		{"openai", func(url string, timeout time.Duration) error {
			o := NewOpenAICompatible(url, "key", "m")
			o.timeout = timeout
			_, err := o.ReadReceipt(context.Background(), receiptInput())
			return err
		}, 1},
		{"anthropic", func(url string, timeout time.Duration) error {
			a := NewAnthropic("sk-test", url, "", "")
			a.timeout = timeout
			_, err := a.ReadReceipt(context.Background(), receiptInput())
			return err
		}, 1 + retries},
	}
	for _, tc := range adapters {
		t.Run(tc.name+" times out", func(t *testing.T) {
			release := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			defer srv.Close()
			defer close(release)

			start := time.Now()
			err := tc.read(srv.URL, budget)
			if elapsed := time.Since(start); elapsed > budget+2*time.Second {
				t.Fatalf("returned after %v, want about %v", elapsed, budget)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("want a deadline error, got %v", err)
			}
		})
		t.Run(tc.name+" retries", func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After-Ms", "1")
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer srv.Close()

			if err := tc.read(srv.URL, requestTimeout); err == nil {
				t.Fatal("want an error from a failing provider")
			}
			if got := calls.Load(); got != tc.attempts {
				t.Fatalf("want %d requests, got %d", tc.attempts, got)
			}
		})
	}
}
