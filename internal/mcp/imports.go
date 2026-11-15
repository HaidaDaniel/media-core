package mcp

import (
	"context"
	"encoding/json"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Release binding and history tool names.
const (
	ToolImportReleases    = "import_releases"
	ToolBindRelease       = "bind_release"
	ToolRebindRelease     = "rebind_release"
	ToolRevokeRelease     = "revoke_release"
	ToolListReleases      = "list_releases"
	ToolListPendingImport = "list_pending_imports"
	ToolGetReleaseBinding = "get_release_binding"
)

// importReleasesSchema takes one Release Watcher version 1.0 envelope as the
// bounded JSON batch; the transport only shapes the envelope as an object and
// the shared service decodes, validates the major and stages the events.
var importReleasesSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"envelope"},
	"properties": map[string]any{
		"envelope":  map[string]any{"type": "object"},
		"namespace": map[string]any{"type": "string"},
	},
}

type importReleasesInput struct {
	Envelope  map[string]any `json:"envelope"`
	Namespace string         `json:"namespace"`
}

var releaseBindingSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "publication_id", "edition_id"},
	"properties": map[string]any{
		"request_id":     map[string]any{"type": "string", "minLength": 1},
		"actor":          actorSchema(),
		"namespace":      map[string]any{"type": "string"},
		"publication_id": map[string]any{"type": "string", "minLength": 1},
		"edition_id":     map[string]any{"type": "string", "minLength": 1},
	},
}

type releaseBindingInput struct {
	RequestID     string     `json:"request_id"`
	Actor         actorInput `json:"actor"`
	Namespace     string     `json:"namespace"`
	PublicationID string     `json:"publication_id"`
	EditionID     string     `json:"edition_id"`
}

var revokeReleaseSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "publication_id"},
	"properties": map[string]any{
		"request_id":     map[string]any{"type": "string", "minLength": 1},
		"actor":          actorSchema(),
		"namespace":      map[string]any{"type": "string"},
		"publication_id": map[string]any{"type": "string", "minLength": 1},
	},
}

type revokeReleaseInput struct {
	RequestID     string     `json:"request_id"`
	Actor         actorInput `json:"actor"`
	Namespace     string     `json:"namespace"`
	PublicationID string     `json:"publication_id"`
}

var listReleasesSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"edition_id"},
	"properties": map[string]any{
		"edition_id": map[string]any{"type": "string", "minLength": 1},
		"limit":      map[string]any{"type": "integer", "minimum": 0, "maximum": 500},
		"cursor":     map[string]any{"type": []any{"null", "string"}},
	},
}

type listReleasesInput struct {
	EditionID string  `json:"edition_id"`
	Limit     int     `json:"limit"`
	Cursor    *string `json:"cursor"`
}

var listPendingImportSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"limit":  map[string]any{"type": "integer", "minimum": 0, "maximum": 500},
		"cursor": map[string]any{"type": []any{"null", "string"}},
	},
}

type listPendingImportInput struct {
	Limit  int     `json:"limit"`
	Cursor *string `json:"cursor"`
}

var getReleaseBindingSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"publication_id"},
	"properties": map[string]any{
		"namespace":      map[string]any{"type": "string"},
		"publication_id": map[string]any{"type": "string", "minLength": 1},
	},
}

type getReleaseBindingInput struct {
	Namespace     string `json:"namespace"`
	PublicationID string `json:"publication_id"`
}

func init() {
	trackTools(ToolImportReleases, ToolBindRelease, ToolRebindRelease, ToolRevokeRelease,
		ToolListReleases, ToolListPendingImport, ToolGetReleaseBinding)
}

// RegisterImportTools installs the release import, binding and history tools.
// Each action routes to the shared service; the transport validates only the
// schema shape and defers every domain rule (envelope major, idempotency, the
// conflict, the projection, the pending queue) to the service.
func RegisterImportTools(s *Server) {
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolImportReleases,
		Description: "Import one Release Watcher version 1.0 envelope (the bounded JSON batch). The envelope is decoded and validated before any write; each event stages idempotently and is reported per item. A canonical-equal event replays; a conflicting identity is a per-item conflict that never replaces the existing row. No edition is inferred from a title; an event stays pending until explicitly bound.",
		InputSchema: importReleasesSchema,
		Annotations: writeAnnotations(),
	}, s.handleImportReleases)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolBindRelease,
		Description: "Explicitly map one confirmed publication to an edition. Idempotent: the same publication bound to the same edition returns the existing active binding. A publication already bound to a different edition is an explicit conflict — never a silent re-point; use rebind_release to correct it. Binds never infer an edition from a title or URL.",
		InputSchema: releaseBindingSchema,
		Annotations: writeAnnotations(),
	}, s.handleBindRelease)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolRebindRelease,
		Description: "Explicitly correct a publication's binding in one step: revoke the current active binding and create its replacement, so the old binding and the raw event history stay intact and only the replacement projects. With no active binding it simply creates one.",
		InputSchema: releaseBindingSchema,
		Annotations: writeAnnotations(),
	}, s.handleRebindRelease)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolRevokeRelease,
		Description: "Revoke one publication's active binding, un-projecting its imported events while keeping the binding row and the raw event history. Revoking a publication with no active binding is a distinguishable not-found.",
		InputSchema: revokeReleaseSchema,
		Annotations: writeAnnotations(),
	}, s.handleRevokeRelease)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolListReleases,
		Description: "Read the bounded release history of one edition: the imported events made visible by every active binding of the edition, oldest first, keyset-paginated. limit 0 uses the default page; the cursor is opaque. Reads never mutate; personal progress and intention are never touched.",
		InputSchema: listReleasesSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleListReleases)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolListPendingImport,
		Description: "Read the bounded pending import queue: imported Release Watcher events with no active binding, oldest first, keyset-paginated. limit 0 uses the default page; the cursor is opaque. Reads never mutate.",
		InputSchema: listPendingImportSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleListPendingImport)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolGetReleaseBinding,
		Description: "Read the active binding of one publication, or an explicit null when there is none.",
		InputSchema: getReleaseBindingSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleGetReleaseBinding)
}

// handleImportReleases is the import_releases tool. The envelope object is
// re-marshaled to the raw bytes and handed to the shared service, which owns
// the decode, the major check and the per-item staging.
func (s *Server) handleImportReleases(ctx context.Context, _ *mcpsdk.CallToolRequest, in importReleasesInput) (*mcpsdk.CallToolResult, app.ImportResult, error) {
	raw, err := json.Marshal(in.Envelope)
	if err != nil {
		return Failure(app.NewAppError(app.ErrInvalidArgument, "marshaling the import envelope: "+err.Error())), app.ImportResult{}, nil
	}
	res, err := s.Service().ImportEvents(ctx, app.ImportRequest{
		RequestID: "mcp-import-releases",
		Namespace: in.Namespace,
		Actor:     app.Actor{Principal: "mcp", Origin: domain.OriginImport},
		Raw:       raw,
	})
	if err != nil {
		return Failure(err), app.ImportResult{}, nil
	}
	return nil, res, nil
}

// handleBindRelease is the bind_release tool.
func (s *Server) handleBindRelease(ctx context.Context, _ *mcpsdk.CallToolRequest, in releaseBindingInput) (*mcpsdk.CallToolResult, app.ReleaseBindingView, error) {
	req := app.BindReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: in.RequestID, Actor: in.Actor.toApp()},
		Namespace:     in.Namespace,
		PublicationID: in.PublicationID,
		EditionID:     in.EditionID,
	}
	view, err := s.Service().BindRelease(ctx, req)
	if err != nil {
		return Failure(err), app.ReleaseBindingView{}, nil
	}
	return nil, view, nil
}

// handleRebindRelease is the rebind_release tool.
func (s *Server) handleRebindRelease(ctx context.Context, _ *mcpsdk.CallToolRequest, in releaseBindingInput) (*mcpsdk.CallToolResult, app.ReleaseBindingView, error) {
	req := app.BindReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: in.RequestID, Actor: in.Actor.toApp()},
		Namespace:     in.Namespace,
		PublicationID: in.PublicationID,
		EditionID:     in.EditionID,
	}
	view, err := s.Service().RebindRelease(ctx, req)
	if err != nil {
		return Failure(err), app.ReleaseBindingView{}, nil
	}
	return nil, view, nil
}

// handleRevokeRelease is the revoke_release tool.
func (s *Server) handleRevokeRelease(ctx context.Context, _ *mcpsdk.CallToolRequest, in revokeReleaseInput) (*mcpsdk.CallToolResult, any, error) {
	req := app.RevokeReleaseRequest{
		MutationBase:  app.MutationBase{RequestID: in.RequestID, Actor: in.Actor.toApp()},
		Namespace:     in.Namespace,
		PublicationID: in.PublicationID,
	}
	if err := s.Service().RevokeRelease(ctx, req); err != nil {
		return Failure(err), nil, nil
	}
	return nil, map[string]any{"status": "revoked", "namespace": req.Namespace, "publication_id": req.PublicationID}, nil
}

// handleListReleases is the list_releases tool.
func (s *Server) handleListReleases(ctx context.Context, _ *mcpsdk.CallToolRequest, in listReleasesInput) (*mcpsdk.CallToolResult, app.ListPayload, error) {
	res, err := s.Service().ListReleases(ctx, app.ReleaseQuery{EditionID: in.EditionID, Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return Failure(err), app.ListPayload{}, nil
	}
	return nil, res, nil
}

// handleListPendingImport is the list_pending_imports tool.
func (s *Server) handleListPendingImport(ctx context.Context, _ *mcpsdk.CallToolRequest, in listPendingImportInput) (*mcpsdk.CallToolResult, app.ListPayload, error) {
	res, err := s.Service().PendingImports(ctx, app.PendingImportQuery{Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return Failure(err), app.ListPayload{}, nil
	}
	return nil, res, nil
}

// handleGetReleaseBinding is the get_release_binding tool.
func (s *Server) handleGetReleaseBinding(ctx context.Context, _ *mcpsdk.CallToolRequest, in getReleaseBindingInput) (*mcpsdk.CallToolResult, any, error) {
	binding, err := s.Service().GetReleaseBinding(ctx, in.Namespace, in.PublicationID)
	if err != nil {
		return Failure(err), nil, nil
	}
	return nil, map[string]any{"active": binding}, nil
}
