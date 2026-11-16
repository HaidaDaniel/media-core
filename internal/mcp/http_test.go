package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMCPHTTPOriginProtection(t *testing.T) {
	s := New(nil, nil)
	handler := s.StreamableHTTP(HTTPOptions{
		SecretReader: func() ([]byte, error) { return []byte("owner-secret"), nil },
	})
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`

	request := func(origin, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8787/mcp", strings.NewReader(initBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+bearer)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	t.Run("hostile origin denied", func(t *testing.T) {
		if rec := request("https://evil.example", "owner-secret"); rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 for a foreign Origin", rec.Code)
		}
	})
	t.Run("origin matching host allowed", func(t *testing.T) {
		if rec := request("http://127.0.0.1:8787", "owner-secret"); rec.Code == http.StatusForbidden {
			t.Fatalf("status = %d, want no origin rejection for a matching Origin", rec.Code)
		}
	})
	t.Run("missing origin allowed", func(t *testing.T) {
		if rec := request("", "owner-secret"); rec.Code == http.StatusForbidden {
			t.Fatalf("status = %d, want no origin rejection for a non-browser client", rec.Code)
		}
	})
	t.Run("wrong bearer still unauthorized", func(t *testing.T) {
		if rec := request("http://127.0.0.1:8787", "wrong-secret"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 for a wrong Bearer token", rec.Code)
		}
	})
}
