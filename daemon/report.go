package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	bridge "github.com/ivandosevv/stef/daemon/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// reportChangesCLI is the same MCP report for agents whose host has not loaded
// stef's tools yet. It neither starts an engine nor edits any files.
func reportChangesCLI(ctx context.Context, endpoint, root string, input io.Reader, output io.Writer) error {
	if err := bridge.ValidateEndpoint(endpoint); err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(input, 1024*1024+1))
	if err != nil {
		return err
	}
	if len(raw) > 1024*1024 {
		return fmt.Errorf("report_changes input exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var arguments bridge.ChangesInput
	if err := decoder.Decode(&arguments); err != nil {
		return fmt.Errorf("invalid report_changes JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("expected one report_changes JSON object")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ready, err := probeCompanion(ctx, endpoint, root)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("start stef for --repo before reporting changes")
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "stef-report", Version: "0.3.0"}, nil)
	httpClient := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	session, err := client.Connect(ctx, &sdk.StreamableClientTransport{Endpoint: strings.TrimSuffix(endpoint, "/") + "/mcp", HTTPClient: httpClient}, nil)
	if err != nil {
		return fmt.Errorf("connect to stef MCP: %w", err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "report_changes", Arguments: arguments})
	if err != nil {
		return fmt.Errorf("report_changes: %w", err)
	}
	if result.IsError {
		return fmt.Errorf("report_changes rejected; verify exact current change IDs and required reasons")
	}
	if result.StructuredContent == nil {
		return fmt.Errorf("report_changes returned no receipt")
	}
	return json.NewEncoder(output).Encode(result.StructuredContent)
}
