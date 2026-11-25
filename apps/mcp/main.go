// Command mcp is the SplitSpends MCP server for local AI assistants. It speaks
// MCP over stdio and calls a SplitSpends server with a personal API token.
//
//	SPLITSPENDS_URL=https://split.example.com SPLITSPENDS_TOKEN=ss_... splitspends-mcp
package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/packages/mcp"
)

var version = "dev"

func main() {
	base, token := os.Getenv("SPLITSPENDS_URL"), os.Getenv("SPLITSPENDS_TOKEN")
	if base == "" || token == "" {
		fmt.Fprintln(os.Stderr, "mcp.config.env.required.not-found: set SPLITSPENDS_URL and SPLITSPENDS_TOKEN (create a token in the app under Account, API tokens)")
		os.Exit(2)
	}
	origin, insecure, err := mcp.Origin(base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp.config.url.invalid: SPLITSPENDS_URL", err)
		os.Exit(2)
	}
	if insecure {
		fmt.Fprintln(os.Stderr, "mcp.config.url.insecure: SPLITSPENDS_URL uses http, so the API token travels unencrypted; use the server's https:// address")
	}
	admin, err := strconv.ParseBool(cmp.Or(os.Getenv("SPLITSPENDS_ADMIN_TOOLS"), "false"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcp.config.env.invalid.SPLITSPENDS_ADMIN_TOOLS: use true or false")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := mcp.NewServer(mcp.NewClient(origin, token, nil), version, mcp.Options{Admin: admin})
	if err := server.Run(ctx, &sdk.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "mcp.server.failed:", err)
		os.Exit(1)
	}
}
