// Package mcp is the MCP stdio transport adapter for media-core. It owns
// protocol framing, capability initialization and the typed result/error
// translation of shared-service calls. Every domain rule stays in the
// shared service (internal/app); nothing in this package re-implements
// one. There are no generic SQL, shell or file tools here and no native
// remote listener: the only transport is stdio, and every diagnostic goes
// to a log stream that is never the protocol output.
package mcp

import (
	"encoding/json"
	"errors"

	"github.com/HaidaDaniel/media-core/internal/app"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Result renders a successful tool payload as the single JSON text
// content of the tool result. The payload is a shared-service view and is
// carried through unchanged.
func Result(payload any) *mcpsdk.CallToolResult {
	data, err := json.Marshal(payload)
	if err != nil {
		return Failure(app.NewAppError(app.ErrInternalError, "rendering the tool result: "+err.Error()))
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}
}

// Failure renders an error as an error tool result. A typed application
// error keeps its stable code, message, details and retryability in the
// transport-visible wire error object shared by every adapter; any other
// error is reported as an internal error. The result is a tool error, not
// a protocol error, so the client model can see it and react.
func Failure(err error) *mcpsdk.CallToolResult {
	var ae *app.AppError
	if errors.As(err, &ae) {
		return errorResult(ae.WireError())
	}
	message := "internal error"
	if err != nil {
		message = err.Error()
	}
	return errorResult(&app.Error{
		Code:    string(app.ErrInternalError),
		Message: message,
	})
}

func errorResult(wire *app.Error) *mcpsdk.CallToolResult {
	data, err := json.Marshal(wire)
	if err != nil {
		data = []byte(`{"code":"internal_error","message":"rendering the error"}`)
	}
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}
}
