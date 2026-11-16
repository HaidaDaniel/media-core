package mcp

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrSecretMissing is reported by an HTTPOptions SecretReader when the
// state directory has no owner secret yet. The handler maps it to the
// distinct "not configured" 401, the same distinction the web login makes.
var ErrSecretMissing = errors.New("mcp: owner secret is not configured")

// HTTPOptions wires the Streamable HTTP handler to a state directory's
// owner secret. The handler owns no secret material of its own: the reader
// is consulted on every request, so a generated secret takes effect
// without a restart and a missing file stays distinguishable from a wrong
// token.
type HTTPOptions struct {
	SecretReader func() ([]byte, error)
}

// StreamableHTTP builds the opt-in Streamable HTTP transport over the same
// in-process service the stdio transport uses: one shared server, one
// stateless session per request, and plain JSON responses. Every request
// must carry the state directory's owner secret as a Bearer token,
// compared in constant time; a missing secret file and a wrong token are
// distinct 401s, and a rejected request never reaches the protocol.
func (s *Server) StreamableHTTP(opts HTTPOptions) http.Handler {
	handler := mcpsdk.NewStreamableHTTPHandler(
		func(*http.Request) *mcpsdk.Server { return s.srv },
		&mcpsdk.StreamableHTTPOptions{
			Stateless:             true,
			JSONResponse:          true,
			CrossOriginProtection: http.NewCrossOriginProtection(),
		})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret, err := opts.SecretReader()
		switch {
		case err == nil:
			if bearer := bearerToken(r); bearer != "" && subtle.ConstantTimeCompare([]byte(bearer), secret) == 1 {
				handler.ServeHTTP(w, r)
				return
			}
			writeAuthError(w, "owner secret does not match")
		case errors.Is(err, ErrSecretMissing):
			writeAuthError(w, "owner secret is not configured; run media-core owner generate")
		default:
			writeAuthError(w, "owner secret is unavailable")
		}
	})
}

// bearerToken extracts the Bearer token from the Authorization header;
// the scheme comparison is case-insensitive per RFC 6750.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		return strings.TrimSpace(auth[len(prefix):])
	}
	return ""
}

// writeAuthError replies 401 with a compact JSON body and a Bearer
// challenge, so a client can tell an authentication failure from a
// protocol error.
func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="media-core"`)
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
