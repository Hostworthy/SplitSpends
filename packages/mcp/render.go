package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
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
	payer := t.names(ctx, e, idx, func(s platform.Share) int64 { return s.Paid })
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
		to := t.names(ctx, e, idx, func(s platform.Share) int64 { return s.Owed })
		return fmt.Sprintf("- %s payment: %s paid %s %s (id %s)\n", e.Date, payer, to, t.c.format(ctx, e.Total, e.Currency), e.ID)
	}
	return fmt.Sprintf("- %s %q: %s paid by %s, %s [%s] (id %s)\n", e.Date, e.Description, t.c.format(ctx, e.Total, e.Currency), payer, mine, e.Category, e.ID)
}

// names lists the people whose amount in e is not zero. With more than one
// each carries the amount, so a bill two people paid is not credited to one.
func (t *tools) names(ctx context.Context, e platform.Expense, idx people, amount func(platform.Share) int64) string {
	var shares []platform.Share
	for _, s := range e.Shares {
		if amount(s) != 0 {
			shares = append(shares, s)
		}
	}
	if len(shares) == 1 {
		return idx.name(shares[0].UserID)
	}
	parts := make([]string, len(shares))
	for i, s := range shares {
		parts[i] = idx.name(s.UserID) + " " + t.c.format(ctx, amount(s), e.Currency)
	}
	return strings.Join(parts, ", ")
}

// page asks a list for limit items starting at offset.
func page(limit, offset int) url.Values {
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	return q
}

// more notes, when a page of n items starting at offset does not reach the
// end of a list of total, which offset shows the next page.
func more(b *strings.Builder, offset, n, total int) {
	if n > 0 && offset+n < total {
		fmt.Fprintf(b, "Showing %d to %d of %d; pass offset %d to see more.\n", offset+1, offset+n, total, offset+n)
	}
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
