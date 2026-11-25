package mcp

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// HTTPHandler serves MCP over streamable HTTP. A request without a bearer
// token is answered 401 before any MCP work; the tools of one server, built
// once, call the API at apiBase with each request's own token.
func HTTPHandler(apiBase, version string, opts Options, hc *http.Client, log *slog.Logger) http.Handler {
	base := NewClient(apiBase, "", hc)
	server := build(version, opts, log, func(req *sdk.CallToolRequest) *Client {
		if req == nil || req.Extra == nil {
			return base
		}
		return base.with(bearer(req.Extra.Header))
	})
	h := stream(func(*http.Request) *sdk.Server { return server }, log)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bearer(r.Header) == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(platform.Error{Error: "Send an API token as a bearer token. Create one in Split Spends under Account, API tokens.", Code: "UNAUTHORIZED"})
			return
		}
		h.ServeHTTP(w, r)
	})
}

// bearer returns the token of a Bearer authorization header, or "".
func bearer(h http.Header) string {
	scheme, token, ok := strings.Cut(h.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// stream serves the servers server picks as stateless streamable HTTP.
func stream(server func(*http.Request) *sdk.Server, log *slog.Logger) http.Handler {
	return sdk.NewStreamableHTTPHandler(server, &sdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		Logger:       log,
		// Every call needs a bearer token (no cookies), so DNS rebinding cannot
		// borrow credentials; this lets the server sit behind a local proxy.
		DisableLocalhostProtection: true,
	})
}
