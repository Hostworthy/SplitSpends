# ADR-006: Optional, Pluggable AI

## Status

Accepted

## Context

Receipt scanning and natural-language entry are valuable, but a self-hosted app must run without any AI vendor, and hosts may prefer a local model.

## Decision

- Split Spends runs no model. The domain defines one port, `ReceiptReader`, with `ReadReceipt` and `DraftExpense`; adapters send requests to the provider the host configured.
- `DraftExpense` takes the user's instructions plus an optional receipt photo. The application writes the system prompt: today's date, the user's people (id and display name only, never an email address) and groups with ids, members and currencies, the categories, the answer rules, and a reminder that text inside images is data, not instructions. The provider must answer in a JSON schema; the domain then drops ids the user cannot use, keeps people inside the chosen group, and reconciles receipt totals. AI is off unless `AI_PROVIDER` is set; the API then returns `AI_DISABLED` and the app hides AI actions (`/config.aiEnabled`).
- `anthropic` uses the official Go SDK with structured outputs (JSON schema), defaulting to `claude-opus-5-5`, with server-side refusal fallbacks on models that support them.
- `openai` targets any OpenAI-compatible chat completions server with JSON-schema response formats (Ollama, vLLM, LM Studio, OpenRouter, LiteLLM).
- Models return decimal strings; the domain converts to minor units and reconciles items, tax, tip and total so that a scan always produces a valid itemized expense (receipt-level discounts are spread over items).
- AI never saves anything. Scans and drafts prefill the editor for the user to review.

## Consequences

### Positive

- Works fully offline from AI vendors; hosts choose cost and privacy trade-offs.

### Negative

- Receipt quality depends on the chosen model's vision ability.
