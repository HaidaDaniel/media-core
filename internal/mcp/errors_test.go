// The typed result/error translation: successful payloads render as
// JSON text content, typed application errors keep their stable code,
// message, details and retryability in the wire error object shared by
// every adapter, and anything else is reported as an internal error.
package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func decodeToolJSON(t *testing.T, res *mcpsdk.CallToolResult) (map[string]any, bool) {
	t.Helper()
	if res == nil {
		t.Fatalf("nil tool result")
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(res.Content))
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("content is not a JSON object: %q (%v)", text.Text, err)
	}
	return payload, res.IsError
}

func TestMCPError(t *testing.T) {
	t.Run("a typed app error keeps code, message, details and retryability", func(t *testing.T) {
		ae := app.NewAppError(app.ErrLocked, "the storage is locked")
		ae.Details = map[string]any{"hint": "retry"}
		payload, isError := decodeToolJSON(t, Failure(ae))
		if !isError {
			t.Fatalf("a typed failure must be a tool error")
		}
		if payload["code"] != "locked" || payload["message"] != "the storage is locked" || payload["retryable"] != true {
			t.Fatalf("wire error lost the typed fields: %v", payload)
		}
		details, ok := payload["details"].(map[string]any)
		if !ok || details["hint"] != "retry" {
			t.Fatalf("wire error lost the details: %v", payload["details"])
		}
	})

	t.Run("a wrapped app error is recognized through errors.As", func(t *testing.T) {
		ae := app.NewAppError(app.ErrConflict, "the edition changed")
		ae = ae.WithCurrentRevision(42)
		wrapped := fmt.Errorf("saving the note: %w", ae)
		payload, isError := decodeToolJSON(t, Failure(wrapped))
		if !isError {
			t.Fatalf("a wrapped failure must be a tool error")
		}
		if payload["code"] != "conflict" || payload["retryable"] != false {
			t.Fatalf("wire error lost the typed fields: %v", payload)
		}
		details, ok := payload["details"].(map[string]any)
		if !ok || details["current_revision"] != float64(42) {
			t.Fatalf("the current revision must travel in details: %v", payload["details"])
		}
	})

	t.Run("a non-app error is reported as an internal error", func(t *testing.T) {
		payload, isError := decodeToolJSON(t, Failure(errors.New("something odd happened")))
		if !isError {
			t.Fatalf("an unclassified failure must be a tool error")
		}
		if payload["code"] != "internal_error" || payload["message"] != "internal error" || payload["retryable"] != false {
			t.Fatalf("unclassified failure must map to a generic internal_error: %v", payload)
		}
		if strings.Contains(fmt.Sprint(payload), "something odd happened") {
			t.Fatalf("raw internal error leaked to the client: %v", payload)
		}
	})

	t.Run("Result renders the payload as JSON text content", func(t *testing.T) {
		payload, isError := decodeToolJSON(t, Result(app.Success(map[string]any{"ok": true})))
		if isError {
			t.Fatalf("a success must not be a tool error")
		}
		if payload["schema_version"] != app.SchemaVersion {
			t.Fatalf("the shared envelope schema version must be preserved: %v", payload)
		}
		if payload["ok"] != true {
			t.Fatalf("the payload must be carried through unchanged: %v", payload)
		}
	})

	t.Run("Result maps an unmarshalable payload to an internal error", func(t *testing.T) {
		// A channel value cannot be marshaled to JSON.
		payload, isError := decodeToolJSON(t, Result(make(chan int)))
		if !isError {
			t.Fatalf("an unrenderable payload must become a tool error")
		}
		if payload["code"] != "internal_error" {
			t.Fatalf("an unrenderable payload must map to internal_error: %v", payload)
		}
	})

	t.Run("Failure never returns a nil result", func(t *testing.T) {
		if Failure(nil) == nil {
			t.Fatalf("Failure(nil) must render an internal error, not nil")
		}
		payload, _ := decodeToolJSON(t, Failure(nil))
		if payload["code"] != "internal_error" {
			t.Fatalf("Failure(nil) must map to internal_error: %v", payload)
		}
	})

	t.Run("Result round-trips a typed view value", func(t *testing.T) {
		view := struct {
			Items []string `json:"items"`
			Total int      `json:"total"`
		}{Items: []string{}, Total: 0}
		res := Result(view)
		text, ok := res.Content[0].(*mcpsdk.TextContent)
		if !ok {
			t.Fatalf("expected text content, got %T", res.Content[0])
		}
		var back struct {
			Items []string `json:"items"`
			Total int      `json:"total"`
		}
		if err := json.Unmarshal([]byte(text.Text), &back); err != nil {
			t.Fatalf("round-trip unmarshal: %v", err)
		}
		if !reflect.DeepEqual(back, view) {
			t.Fatalf("round-trip mismatch: got %+v, want %+v", back, view)
		}
	})
}

func TestErrorUnclassifiedMessageIsGeneric(t *testing.T) {
	payload, isError := decodeToolJSON(t, Failure(errors.New("SQLITE_CANTOPEN: /state/owner-secret.db")))
	if !isError {
		t.Fatal("an unclassified failure must be a tool error")
	}
	if payload["code"] != "internal_error" || payload["message"] != "internal error" {
		t.Fatalf("unclassified failure must map to a generic internal_error: %v", payload)
	}
	if strings.Contains(fmt.Sprint(payload), "SQLITE_CANTOPEN") || strings.Contains(fmt.Sprint(payload), "/state/") {
		t.Fatalf("raw internal detail leaked to the client: %v", payload)
	}
}
