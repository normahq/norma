package mcpcmd

import (
	"context"
	"io"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func PingPongCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "ping-pong",
		Short:        "Run a ping-pong MCP server over stdio",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPingPongServer(cmd.Context(), os.Stdin, os.Stdout, os.Stderr)
		},
	}
}

type pingInput struct {
	Message string `json:"message"`
}

type pingOutput struct {
	Reply string `json:"reply"`
}

func runPingPongServer(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "norma-playground-ping-pong",
			Version: "1.0.0",
		},
		nil,
	)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "Responds with pong and the original message",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input pingInput) (*mcp.CallToolResult, pingOutput, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "pong: " + input.Message},
			},
		}, pingOutput{Reply: "pong: " + input.Message}, nil
	})

	return server.Run(ctx, &mcp.StdioTransport{})
}
