// Command mcp is the SplitSpends MCP server for local AI assistants. It speaks
// MCP over stdio and calls a SplitSpends server with a personal API token.
//
//	SPLITSPENDS_URL=https://split.example.com SPLITSPENDS_TOKEN=ss_... splitspends-mcp
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/backend/internal/mcp"
)

var version = "dev"

func main() {
	base, token := os.Getenv("SPLITSPENDS_URL"), os.Getenv("SPLITSPENDS_TOKEN")
	if base == "" || token == "" {
		fmt.Fprintln(os.Stderr, "mcp.config.env.required.not-found: set SPLITSPENDS_URL and SPLITSPENDS_TOKEN (create a token under Settings, API tokens)")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := mcp.NewServer(mcp.NewClient(base, token, nil), version)
	if err := server.Run(ctx, &sdk.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "mcp.server.failed:", err)
		os.Exit(1)
	}
}
