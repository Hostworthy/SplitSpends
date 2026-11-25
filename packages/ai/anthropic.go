package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// DefaultAnthropicModel is used when AI_MODEL is empty.
const DefaultAnthropicModel = "claude-opus-5-5"

// Models that accept server-side refusal fallbacks.
var fallbackModels = map[string]bool{
	"claude-fable-5-2": true, "claude-fable-5-1": true, "claude-opus-5-5": true, "claude-opus-5": true, "claude-sonnet-5-5": true,
}

// Anthropic reads receipts with Claude.
type Anthropic struct {
	client  anthropic.Client
	model   string
	effort  string
	timeout time.Duration
}

// NewAnthropic builds an Anthropic reader. effort may be empty.
func NewAnthropic(apiKey, baseURL, model, effort string) *Anthropic {
	// The SDK otherwise allows each attempt up to ten minutes and retries
	// twice; ask bounds the whole call instead.
	opts := []option.RequestOption{option.WithAPIKey(apiKey), option.WithMaxRetries(retries)}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if model == "" {
		model = DefaultAnthropicModel
	}
	return &Anthropic{client: anthropic.NewClient(opts...), model: model, effort: effort, timeout: requestTimeout}
}

func (a *Anthropic) ask(ctx context.Context, system string, content []anthropic.BetaContentBlockParamUnion, schema map[string]any) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	params := anthropic.BetaMessageNewParams{
		Model:     a.model,
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: system}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(content...)},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
	}
	if a.effort != "" {
		params.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(a.effort)
	}
	if fallbackModels[a.model] {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}
	resp, err := a.client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden:
				return "", &rejection{key: "ai.anthropic.auth.failed", Status: apiErr.StatusCode}
			case http.StatusTooManyRequests:
				return "", errs.RateLimited("The AI provider is busy. Try again in a minute.")
			}
			// The SDK's message carries the provider's answer, which can
			// echo the request.
			return "", &rejection{key: "ai.anthropic.request.rejected", Status: apiErr.StatusCode}
		}
		return "", fmt.Errorf("ai.anthropic.request.failed: %w", err)
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return "", errs.Validation("file", "The AI model declined to read this input.")
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	return text.String(), nil
}

// ReadReceipt implements ports.ReceiptReader.
func (a *Anthropic) ReadReceipt(ctx context.Context, in ports.ReceiptInput) (ports.ReceiptExtract, error) {
	if err := imageSupported(in.ContentType); err != nil {
		return ports.ReceiptExtract{}, err
	}
	image := anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
		Data:      base64.StdEncoding.EncodeToString(in.Image),
		MediaType: anthropic.BetaBase64ImageSourceMediaType(in.ContentType),
	})
	raw, err := a.ask(ctx, receiptInstructions(in), []anthropic.BetaContentBlockParamUnion{image, anthropic.NewBetaTextBlock("Read this receipt.")}, receiptSchema)
	if err != nil {
		return ports.ReceiptExtract{}, err
	}
	return parseReceipt(raw)
}

// DraftExpense implements ports.ReceiptReader.
func (a *Anthropic) DraftExpense(ctx context.Context, in ports.DraftInput) (ports.DraftExtract, error) {
	content := []anthropic.BetaContentBlockParamUnion{}
	if len(in.Image) > 0 {
		if err := imageSupported(in.ContentType); err != nil {
			return ports.DraftExtract{}, err
		}
		content = append(content, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      base64.StdEncoding.EncodeToString(in.Image),
			MediaType: anthropic.BetaBase64ImageSourceMediaType(in.ContentType),
		}))
	}
	content = append(content, anthropic.NewBetaTextBlock(instructions(in)))
	raw, err := a.ask(ctx, draftInstructions(in), content, draftSchema)
	if err != nil {
		return ports.DraftExtract{}, err
	}
	return parseDraft(raw)
}
