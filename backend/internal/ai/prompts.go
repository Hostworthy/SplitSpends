// Package ai implements ports.ReceiptReader with Anthropic's Claude API or any
// OpenAI-compatible endpoint (Ollama, vLLM, OpenRouter, LiteLLM and so on).
package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/atb00ker/splitspends/backend/internal/core/ports"
)

const receiptPrompt = `You read photos of receipts and bills for an expense-splitting app.
Extract every purchased line item exactly as printed, with its line total (quantity times unit price).
Rules:
- Write every money amount as a plain decimal string with a dot, no currency symbol or thousands separator, for example "12.50".
- Put taxes, VAT and service charges that are listed separately into "tax". Put tips or gratuity into "tip". Do not list them as items.
- If a discount applies to a single item, subtract it from that item. Leave order-level discounts out; the app reconciles them using "total".
- "total" is the final amount paid. "subtotal" is the sum of items before tax and tip.
- "currency" is the ISO 4217 code shown or implied by the receipt; if unclear use %s.
- "date" is YYYY-MM-DD or an empty string when not printed.
- "category" must be one of: %s.
If the image is not a receipt, return an empty items list and empty amounts.`

const draftPrompt = `You turn a short note into an expense for a bill-splitting app. Today is %s and the default currency is %s.
The person writing the note is "me" (id %s, name %s).
People they can split with:
%s
Rules:
- "amount" is a plain decimal string such as "60" or "12.50" with no symbol.
- "payerId" is the id of whoever paid; use me when the note does not say.
- "participantIds" are everyone sharing the cost, including me unless the note clearly excludes me. "split with Sam" means me and Sam.
- Match people by name or email, tolerating nicknames and typos; only use ids from the list.
- Put any mentioned person you cannot match into "unmatched".
- "date" is YYYY-MM-DD; resolve words like "yesterday" against today.
- "category" must be one of: %s.
- "description" is a short title such as "Dinner at Lua".`

func receiptInstructions(in ports.ReceiptInput) string {
	currency := in.Currency
	if currency == "" {
		currency = "the most likely currency"
	}
	return fmt.Sprintf(receiptPrompt, currency, strings.Join(in.Categories, ", "))
}

func draftInstructions(in ports.DraftInput) string {
	var people strings.Builder
	for _, p := range in.People {
		fmt.Fprintf(&people, "- id %s: %s <%s>\n", p.ID, p.Name, p.Email)
	}
	if people.Len() == 0 {
		people.WriteString("- (nobody yet)\n")
	}
	return fmt.Sprintf(draftPrompt, in.Today, in.Currency, in.Me.ID, in.Me.Name, people.String(), strings.Join(in.Categories, ", "))
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func object(props map[string]any) map[string]any {
	required := make([]string, 0, len(props))
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

var receiptSchema = object(map[string]any{
	"merchant": str("Store or restaurant name"),
	"date":     str("YYYY-MM-DD or empty"),
	"currency": str("ISO 4217 code"),
	"category": str("Category id"),
	"subtotal": str("Decimal amount"),
	"tax":      str("Decimal amount"),
	"tip":      str("Decimal amount"),
	"total":    str("Decimal amount"),
	"items": map[string]any{"type": "array", "items": object(map[string]any{
		"name":     str("Item name"),
		"quantity": map[string]any{"type": "integer"},
		"amount":   str("Line total as a decimal amount"),
	})},
})

var draftSchema = object(map[string]any{
	"description":    str("Short title"),
	"amount":         str("Decimal amount"),
	"currency":       str("ISO 4217 code"),
	"date":           str("YYYY-MM-DD"),
	"category":       str("Category id"),
	"payerId":        str("Id of the payer"),
	"participantIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	"unmatched":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
})

type receiptJSON struct {
	Merchant string `json:"merchant"`
	Date     string `json:"date"`
	Currency string `json:"currency"`
	Category string `json:"category"`
	Subtotal string `json:"subtotal"`
	Tax      string `json:"tax"`
	Tip      string `json:"tip"`
	Total    string `json:"total"`
	Items    []struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
		Amount   string `json:"amount"`
	} `json:"items"`
}

func parseReceipt(raw string) (ports.ReceiptExtract, error) {
	var r receiptJSON
	if err := json.Unmarshal([]byte(stripFence(raw)), &r); err != nil {
		return ports.ReceiptExtract{}, fmt.Errorf("ai.receipt.parse.failed: %w", err)
	}
	out := ports.ReceiptExtract{Merchant: r.Merchant, Date: r.Date, Currency: r.Currency, Category: r.Category,
		Subtotal: r.Subtotal, Tax: r.Tax, Tip: r.Tip, Total: r.Total}
	for _, it := range r.Items {
		out.Items = append(out.Items, ports.ExtractItem{Name: it.Name, Quantity: it.Quantity, Amount: it.Amount})
	}
	return out, nil
}

type draftJSON struct {
	Description    string   `json:"description"`
	Amount         string   `json:"amount"`
	Currency       string   `json:"currency"`
	Date           string   `json:"date"`
	Category       string   `json:"category"`
	PayerID        string   `json:"payerId"`
	ParticipantIDs []string `json:"participantIds"`
	Unmatched      []string `json:"unmatched"`
}

func parseDraft(raw string) (ports.DraftExtract, error) {
	var d draftJSON
	if err := json.Unmarshal([]byte(stripFence(raw)), &d); err != nil {
		return ports.DraftExtract{}, fmt.Errorf("ai.draft.parse.failed: %w", err)
	}
	return ports.DraftExtract(d), nil
}

// stripFence tolerates models that wrap JSON in a Markdown fence.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
