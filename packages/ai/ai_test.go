package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
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
	in := draftInput()
	in.ContentType = "image/heic"
	if _, err := NewOpenAICompatible("http://127.0.0.1:1", "", "m").DraftExpense(context.Background(), in); err == nil ||
		!strings.Contains(err.Error(), "JPEG, PNG and WebP") {
		t.Fatalf("want a friendly format error, got %v", err)
	}
}
