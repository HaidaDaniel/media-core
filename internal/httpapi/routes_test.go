package httpapi

import (
	"errors"
	"strings"
	"testing"

	"github.com/HaidaDaniel/media-core/internal/app"
)

func TestErrorCatalogTypedHidesInternalDetail(t *testing.T) {
	raw := "SQLITE_CANTOPEN: unable to open /state/owner-secret.db"
	got := catalogTyped(errors.New(raw))
	if got.Code != app.ErrStorageError {
		t.Fatalf("code = %s, want storage_error", got.Code)
	}
	if strings.Contains(got.Message, "SQLITE_CANTOPEN") || strings.Contains(got.Message, "/state/") {
		t.Fatalf("raw internal detail leaked into the client message: %q", got.Message)
	}
	if got.Message != "storage operation failed" {
		t.Fatalf("message = %q, want the generic storage message", got.Message)
	}

	typed := app.NewAppError(app.ErrConflict, "typed client message")
	if settled := catalogTyped(typed); settled != typed || settled.Message != "typed client message" {
		t.Fatalf("typed application errors must pass through unchanged: %+v", settled)
	}
}
