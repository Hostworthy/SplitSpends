package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

// OpenAICompatible talks to any server implementing the OpenAI chat
// completions API with JSON-schema response formats, such as Ollama, vLLM,
// LM Studio, OpenRouter or LiteLLM.
type OpenAICompatible struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// NewOpenAICompatible builds a reader. baseURL ends before /chat/completions,
// for example http://localhost:11434/v1.
func NewOpenAICompatible(baseURL, apiKey, model string) *OpenAICompatible {
	return &OpenAICompatible{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model, http: &http.Client{Timeout: 3 * time.Minute}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

func (o *OpenAICompatible) ask(ctx context.Context, system string, user any, name string, schema map[string]any) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    o.model,
		"messages": []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": name, "strict": true, "schema": schema,
		}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai.openai.request.failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", errs.RateLimited("The AI provider is busy. Try again in a minute.")
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ai.openai.status.%d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", fmt.Errorf("ai.openai.response.invalid: %s", truncate(string(raw), 300))
	}
	if parsed.Choices[0].Message.Refusal != "" {
		return "", errs.Validation("file", "The AI model declined to read this input.")
	}
	return parsed.Choices[0].Message.Content, nil
}

// ReadReceipt implements ports.ReceiptReader.
func (o *OpenAICompatible) ReadReceipt(ctx context.Context, in ports.ReceiptInput) (ports.ReceiptExtract, error) {
	url := "data:" + in.ContentType + ";base64," + base64.StdEncoding.EncodeToString(in.Image)
	content := []map[string]any{
		{"type": "image_url", "image_url": map[string]any{"url": url}},
		{"type": "text", "text": "Read this receipt."},
	}
	raw, err := o.ask(ctx, receiptInstructions(in), content, "receipt", receiptSchema)
	if err != nil {
		return ports.ReceiptExtract{}, err
	}
	return parseReceipt(raw)
}

// DraftExpense implements ports.ReceiptReader.
func (o *OpenAICompatible) DraftExpense(ctx context.Context, in ports.DraftInput) (ports.DraftExtract, error) {
	var user any = instructions(in)
	if len(in.Image) > 0 {
		if err := imageSupported(in.ContentType); err != nil {
			return ports.DraftExtract{}, err
		}
		url := "data:" + in.ContentType + ";base64," + base64.StdEncoding.EncodeToString(in.Image)
		user = []map[string]any{
			{"type": "image_url", "image_url": map[string]any{"url": url}},
			{"type": "text", "text": instructions(in)},
		}
	}
	raw, err := o.ask(ctx, draftInstructions(in), user, "expense_draft", draftSchema)
	if err != nil {
		return ports.DraftExtract{}, err
	}
	return parseDraft(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
