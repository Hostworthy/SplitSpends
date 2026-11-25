package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/platform"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Administration tools. The API decides who may use them: managers can
// list and decide requests, administrators can do everything.
func (t *tools) register(s *sdk.Server) {
	readOnly := &sdk.ToolAnnotations{ReadOnlyHint: true}
	destructive := true
	add := func(name, desc string, a *sdk.ToolAnnotations) *sdk.Tool {
		return &sdk.Tool{Name: name, Description: desc, Annotations: a}
	}
	sdk.AddTool(s, add("admin_get_signup", "Administrators: the sign-up mode (open, invite, approval or closed) and the allowed list of emails and @domains.", readOnly), t.getSignup)
	sdk.AddTool(s, add("admin_set_signup", "Administrators: change the sign-up mode and, when given, replace the allowed list. Confirm with the user first.", nil), t.setSignup)
	sdk.AddTool(s, add("admin_list_requests", "Administrators and managers: people waiting for approval (or already approved or declined).", readOnly), t.listRequests)
	sdk.AddTool(s, add("admin_decide_request", "Administrators and managers: approve or decline someone waiting to join. Confirm with the user first.", nil), t.decideRequest)
	sdk.AddTool(s, add("admin_list_accounts", "Administrators: accounts with their role and status, filtered by status or a name or email search.", readOnly), t.listAccounts)
	sdk.AddTool(s, add("admin_set_role", "Administrators: make an account a user, manager or admin. Confirm with the user first.", nil), t.setRole)
	sdk.AddTool(s, add("admin_disable_account", "Administrators: disable an account. It is signed out everywhere and its email is removed (only a hash is kept); its name stays on past expenses. Confirm with the user first.",
		&sdk.ToolAnnotations{DestructiveHint: &destructive}), t.disableAccount)
	sdk.AddTool(s, add("admin_enable_account", "Administrators: let a disabled account sign in again with its old email.", nil), t.enableAccount)
}

func (t *tools) getSignup(ctx context.Context, _ *sdk.CallToolRequest, _ empty) (*sdk.CallToolResult, any, error) {
	s, err := get[platform.SignupSettings](ctx, t.c, "/admin/signup", nil)
	if err != nil {
		return fail(err)
	}
	return text(signupText(s)), nil, nil
}

func signupText(s platform.SignupSettings) string {
	allowed := "nobody"
	if len(s.Allowed) > 0 {
		allowed = strings.Join(s.Allowed, ", ")
	}
	return fmt.Sprintf("Sign-up mode: %s\nAllowed list: %s", s.Mode, allowed)
}

type signupArgs struct {
	Mode    string   `json:"mode" jsonschema:"open, invite, approval or closed"`
	Allowed []string `json:"allowed,omitempty" jsonschema:"replaces the allowed list (emails or @domain); omit to keep it"`
}

func (t *tools) setSignup(ctx context.Context, _ *sdk.CallToolRequest, in signupArgs) (*sdk.CallToolResult, any, error) {
	allowed := in.Allowed
	if allowed == nil {
		current, err := get[platform.SignupSettings](ctx, t.c, "/admin/signup", nil)
		if err != nil {
			return fail(err)
		}
		allowed = current.Allowed
	}
	s, err := send[platform.SignupSettings](ctx, t.c, http.MethodPut, "/admin/signup", platform.SignupSettingsInput{Mode: in.Mode, Allowed: allowed})
	if err != nil {
		return fail(err)
	}
	return text("Saved.\n" + signupText(s)), nil, nil
}

type requestsArgs struct {
	Status string `json:"status,omitempty" jsonschema:"pending (default), approved or declined"`
}

func (t *tools) listRequests(ctx context.Context, _ *sdk.CallToolRequest, in requestsArgs) (*sdk.CallToolResult, any, error) {
	q := url.Values{"limit": {"200"}}
	if in.Status != "" {
		q.Set("status", in.Status)
	}
	list, err := getList[platform.SignupRequest](ctx, t.c, "/admin/requests", q)
	if err != nil {
		return fail(err)
	}
	if len(list.Items) == 0 {
		return text("No requests."), nil, nil
	}
	var b strings.Builder
	for _, r := range list.Items {
		name := r.Name
		if name == "" {
			name = "(no name)"
		}
		fmt.Fprintf(&b, "- %s <%s>, %s since %s (id %s)\n", name, r.Email, r.Status, r.RequestedAt[:10], r.ID)
	}
	if list.Total > len(list.Items) {
		fmt.Fprintf(&b, "Showing the newest %d of %d.\n", len(list.Items), list.Total)
	}
	return text(b.String()), nil, nil
}

type decideArgs struct {
	RequestID string `json:"requestId"`
	Decision  string `json:"decision" jsonschema:"approve or decline"`
}

func (t *tools) decideRequest(ctx context.Context, _ *sdk.CallToolRequest, in decideArgs) (*sdk.CallToolResult, any, error) {
	action := strings.ToLower(strings.TrimSpace(in.Decision))
	if action != "approve" && action != "decline" {
		return text("decision must be approve or decline"), nil, nil
	}
	r, err := send[platform.SignupRequest](ctx, t.c, http.MethodPost, "/admin/requests/"+url.PathEscape(in.RequestID)+"/"+action, nil)
	if err != nil {
		return fail(err)
	}
	return text(fmt.Sprintf("%s <%s> is now %s.", r.Name, r.Email, r.Status)), nil, nil
}

type accountsArgs struct {
	Status string `json:"status,omitempty" jsonschema:"active, invited or disabled"`
	Query  string `json:"query,omitempty" jsonschema:"part of a name or email"`
}

func (t *tools) listAccounts(ctx context.Context, _ *sdk.CallToolRequest, in accountsArgs) (*sdk.CallToolResult, any, error) {
	q := url.Values{"limit": {"200"}}
	if in.Status != "" {
		q.Set("status", in.Status)
	}
	if in.Query != "" {
		q.Set("q", in.Query)
	}
	list, err := getList[platform.Account](ctx, t.c, "/admin/accounts", q)
	if err != nil {
		return fail(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d accounts\n", list.Total)
	for _, a := range list.Items {
		fmt.Fprintf(&b, "- %s <%s>: %s, %s (id %s)\n", a.Name, email(a.Email), a.Role, a.Status, a.ID)
	}
	return text(b.String()), nil, nil
}

type roleArgs struct {
	UserID string `json:"userId"`
	Role   string `json:"role" jsonschema:"user, manager or admin"`
}

func (t *tools) setRole(ctx context.Context, _ *sdk.CallToolRequest, in roleArgs) (*sdk.CallToolResult, any, error) {
	a, err := send[platform.Account](ctx, t.c, http.MethodPatch, "/admin/accounts/"+url.PathEscape(in.UserID), platform.RoleInput{Role: in.Role})
	if err != nil {
		return fail(err)
	}
	return text(fmt.Sprintf("%s is now %s.", a.Name, a.Role)), nil, nil
}

type accountArgs struct {
	UserID string `json:"userId"`
}

func (t *tools) disableAccount(ctx context.Context, _ *sdk.CallToolRequest, in accountArgs) (*sdk.CallToolResult, any, error) {
	a, err := send[platform.Account](ctx, t.c, http.MethodPost, "/admin/accounts/"+url.PathEscape(in.UserID)+"/disable", nil)
	if err != nil {
		return fail(err)
	}
	return text(a.Name + " is disabled and signed out everywhere."), nil, nil
}

func (t *tools) enableAccount(ctx context.Context, _ *sdk.CallToolRequest, in accountArgs) (*sdk.CallToolResult, any, error) {
	a, err := send[platform.Account](ctx, t.c, http.MethodPost, "/admin/accounts/"+url.PathEscape(in.UserID)+"/enable", nil)
	if err != nil {
		return fail(err)
	}
	return text(a.Name + " can sign in again with their old email."), nil, nil
}
