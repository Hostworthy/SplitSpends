package mcp

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func text(s string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: s}}}
}

// people indexes quoted names by id from friends and groups.
type people map[string]string

func (p people) name(id string) string {
	if n, ok := p[id]; ok {
		return n
	}
	return id
}

func (t *tools) balances(ctx context.Context, bs []platform.Balance) string {
	if len(bs) == 0 {
		return "settled up"
	}
	parts := make([]string, 0, len(bs))
	for _, b := range bs {
		parts = append(parts, t.c.format(ctx, b.Amount, b.Currency))
	}
	return strings.Join(parts, ", ")
}

func (t *tools) line(ctx context.Context, e platform.Expense, idx people) string {
	payer := ""
	var best int64
	for _, s := range e.Shares {
		if s.Paid > best {
			best, payer = s.Paid, s.UserID
		}
	}
	var mine string
	switch {
	case e.MyBalance > 0:
		mine = "you lent " + t.c.format(ctx, e.MyBalance, e.Currency)
	case e.MyBalance < 0:
		mine = "you borrowed " + t.c.format(ctx, -e.MyBalance, e.Currency)
	default:
		mine = "not involved or even"
	}
	if e.Kind == "payment" {
		to := ""
		for _, s := range e.Shares {
			if s.Owed > 0 {
				to = s.UserID
			}
		}
		return fmt.Sprintf("- %s payment: %s paid %s %s (id %s)\n", e.Date, idx.name(payer), idx.name(to), t.c.format(ctx, e.Total, e.Currency), e.ID)
	}
	return fmt.Sprintf("- %s %q: %s paid by %s, %s [%s] (id %s)\n", e.Date, e.Description, t.c.format(ctx, e.Total, e.Currency), idx.name(payer), mine, e.Category, e.ID)
}

// day is the date part of an RFC 3339 timestamp, or ts itself when it is
// too short to have one.
func day(ts string) string {
	if len(ts) < len("2006-01-02") {
		return ts
	}
	return ts[:len("2006-01-02")]
}

func email(e *string) string {
	if e == nil {
		return "no email"
	}
	return *e
}
