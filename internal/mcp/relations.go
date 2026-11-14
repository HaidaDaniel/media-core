package mcp

import (
	"context"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Relation and origin tool names.
const (
	ToolListRelated   = "list_related"
	ToolWriteRelation = "write_relation"
	ToolWriteOrigin   = "write_origin"
)

// write_relation / write_origin actions.
const (
	RelationPropose = "propose"
	RelationConfirm = "confirm"
	RelationRevoke  = "revoke"
)

func relationScopeSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{string(domain.ScopeWork), string(domain.ScopeEdition)},
	}
}

func relationTypeSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{
			string(domain.RelationAdaptationOf), string(domain.RelationTranslationOf),
			string(domain.RelationAlternativeVersion), string(domain.RelationSameUnderlyingWork),
			string(domain.RelationSequelOf), string(domain.RelationSideStoryOf),
			string(domain.RelationRelated),
		},
	}
}

func originKindSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{
			string(domain.OriginOriginal), string(domain.OriginAnimeOriginal),
			string(domain.OriginUnknown),
		},
	}
}

var listRelatedSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"scope", "entity_id"},
	"properties": map[string]any{
		"scope":     relationScopeSchema(),
		"entity_id": map[string]any{"type": "string", "minLength": 1},
		"depth":     map[string]any{"type": "integer", "minimum": 0, "maximum": 3},
	},
}

var writeRelationSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "action"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"action":            map[string]any{"type": "string", "enum": []any{RelationPropose, RelationConfirm, RelationRevoke}},
		"scope":             map[string]any{"type": []any{"null", "string"}, "enum": []any{nil, string(domain.ScopeWork), string(domain.ScopeEdition)}},
		"type":              map[string]any{"type": []any{"null", "string"}, "enum": []any{nil, string(domain.RelationAdaptationOf), string(domain.RelationTranslationOf), string(domain.RelationAlternativeVersion), string(domain.RelationSameUnderlyingWork), string(domain.RelationSequelOf), string(domain.RelationSideStoryOf), string(domain.RelationRelated)}},
		"from_id":           map[string]any{"type": []any{"null", "string"}},
		"to_id":             map[string]any{"type": []any{"null", "string"}},
		"provenance":        map[string]any{"type": []any{"null", "string"}},
		"confidence":        map[string]any{"type": []any{"null", "number"}, "minimum": 0, "maximum": 1},
		"replaces_id":       map[string]any{"type": []any{"null", "string"}},
		"relation_id":       map[string]any{"type": []any{"null", "string"}},
		"confirmation":      map[string]any{"type": "boolean"},
		"user_instruction":  map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var writeOriginSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "action"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"action":            map[string]any{"type": "string", "enum": []any{RelationPropose, RelationConfirm, RelationRevoke}},
		"edition_id":        map[string]any{"type": []any{"null", "string"}},
		"kind":              map[string]any{"type": []any{"null", "string"}, "enum": []any{nil, string(domain.OriginOriginal), string(domain.OriginAnimeOriginal), string(domain.OriginUnknown)}},
		"provenance":        map[string]any{"type": []any{"null", "string"}},
		"replaces_id":       map[string]any{"type": []any{"null", "string"}},
		"claim_id":          map[string]any{"type": []any{"null", "string"}},
		"confirmation":      map[string]any{"type": "boolean"},
		"user_instruction":  map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

type listRelatedInput struct {
	Scope    string `json:"scope"`
	EntityID string `json:"entity_id"`
	Depth    int    `json:"depth"`
}

type writeRelationInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	Action           string     `json:"action"`
	Scope            *string    `json:"scope"`
	Type             *string    `json:"type"`
	FromID           *string    `json:"from_id"`
	ToID             *string    `json:"to_id"`
	Provenance       *string    `json:"provenance"`
	Confidence       *float64   `json:"confidence"`
	ReplacesID       *string    `json:"replaces_id"`
	RelationID       *string    `json:"relation_id"`
	Confirmation     bool       `json:"confirmation"`
	UserInstruction  *string    `json:"user_instruction"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type writeOriginInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	Action           string     `json:"action"`
	EditionID        *string    `json:"edition_id"`
	Kind             *string    `json:"kind"`
	Provenance       *string    `json:"provenance"`
	ReplacesID       *string    `json:"replaces_id"`
	ClaimID          *string    `json:"claim_id"`
	Confirmation     bool       `json:"confirmation"`
	UserInstruction  *string    `json:"user_instruction"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

func init() {
	trackTools(ToolListRelated, ToolWriteRelation, ToolWriteOrigin)
}

// RegisterRelationTools installs the relation and origin tools. Proposals
// are the default for agent-found edges: a propose creates a proposed
// claim with source provenance, and a confidence number is never
// confirmation. A confirm is an explicit user decision guarded by the
// confirmation flag and the user-instruction context; the cycle guard and
// every domain rule stay in the shared service.
func RegisterRelationTools(s *Server) {
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolListRelated,
		Description: "Traverse the confirmed relation graph from one work or edition up to three hops deep (depth 0 or 1 uses the default single hop). Edges carry their relation type and direction; proposed edges are separately labelled and are not traversed as confirmed.",
		InputSchema: listRelatedSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleListRelated)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolWriteRelation,
		Description: "Write one typed relation between two works or editions. Actions: propose (scope, type, from_id, to_id and source provenance; the edge is created proposed — an agent guess is never a confirmed fact, and a confidence number is never confirmation), confirm (relation_id plus confirmation true and the user_instruction context; the cycle check runs at confirmation) and revoke (relation_id; the row keeps its history). expected_revision comes from the current view; a stale revision is a conflict.",
		InputSchema: writeRelationSchema,
		Annotations: writeAnnotations(),
	}, s.handleWriteRelation)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolWriteOrigin,
		Description: "Assert one explicit origin claim on an edition. Actions: propose (edition_id, kind and source provenance; the claim is created proposed), confirm (edition_id, claim_id plus confirmation true and the user_instruction context) and revoke (edition_id and claim_id; the row keeps its history). An unknown origin is the default absence; no originality is ever inferred from a media kind or a provider. expected_revision comes from the current edition view; a stale revision is a conflict.",
		InputSchema: writeOriginSchema,
		Annotations: writeAnnotations(),
	}, s.handleWriteOrigin)
}

// handleListRelated is the list_related tool.
func (s *Server) handleListRelated(_ context.Context, _ *mcpsdk.CallToolRequest, in listRelatedInput) (*mcpsdk.CallToolResult, app.RelatedView, error) {
	res, err := s.Service().Related(context.Background(), domain.RelationScope(in.Scope), in.EntityID, in.Depth)
	if err != nil {
		return Failure(err), app.RelatedView{}, nil
	}
	return nil, res, nil
}

// confirmationGuard checks the explicit intent guard for a confirm
// action: the caller must set confirmation true and supply the
// user-instruction context. A confidence number never satisfies it.
func confirmationGuard(in struct {
	Confirmation    bool
	UserInstruction *string
}) error {
	if !in.Confirmation {
		return app.NewAppError(app.ErrInvalidArgument, "confirm requires an explicit confirmation true")
	}
	if in.UserInstruction == nil || *in.UserInstruction == "" {
		return app.NewAppError(app.ErrInvalidArgument, "confirm requires the user_instruction context")
	}
	return nil
}

// handleWriteRelation is the write_relation tool. The action-to-fields
// shape and the confirmation guard are transport concerns; the cycle
// guard, the journal and the revision guard are the shared service's.
func (s *Server) handleWriteRelation(ctx context.Context, _ *mcpsdk.CallToolRequest, in writeRelationInput) (*mcpsdk.CallToolResult, app.RelationView, error) {
	base := app.MutationBase{
		RequestID:        in.RequestID,
		Actor:            in.Actor.toApp(),
		ExpectedRevision: in.ExpectedRevision,
	}
	invalid := func(message string) (*mcpsdk.CallToolResult, app.RelationView, error) {
		return Failure(app.NewAppError(app.ErrInvalidArgument, message)), app.RelationView{}, nil
	}
	switch in.Action {
	case RelationPropose:
		if in.Scope == nil || in.Type == nil || in.FromID == nil || in.ToID == nil || in.Provenance == nil {
			return invalid("propose requires scope, type, from_id, to_id and provenance")
		}
		state := domain.ClaimProposed
		req := app.AddRelationRequest{
			MutationBase: base,
			Scope:        domain.RelationScope(*in.Scope),
			Type:         domain.RelationType(*in.Type),
			FromID:       *in.FromID,
			ToID:         *in.ToID,
			State:        state,
			Provenance:   *in.Provenance,
			Confidence:   in.Confidence,
			ReplacesID:   in.ReplacesID,
		}
		res, err := s.Service().AddRelation(ctx, req)
		if err != nil {
			return Failure(err), app.RelationView{}, nil
		}
		return nil, res, nil
	case RelationConfirm:
		if in.RelationID == nil {
			return invalid("confirm requires relation_id")
		}
		if err := confirmationGuard(struct {
			Confirmation    bool
			UserInstruction *string
		}{in.Confirmation, in.UserInstruction}); err != nil {
			return Failure(err), app.RelationView{}, nil
		}
		req := app.ConfirmRelationRequest{
			MutationBase: base,
			RelationID:   *in.RelationID,
		}
		res, err := s.Service().ConfirmRelation(ctx, req)
		if err != nil {
			return Failure(err), app.RelationView{}, nil
		}
		return nil, res, nil
	case RelationRevoke:
		if in.RelationID == nil {
			return invalid("revoke requires relation_id")
		}
		req := app.RevokeRelationRequest{
			MutationBase: base,
			RelationID:   *in.RelationID,
		}
		res, err := s.Service().RevokeRelation(ctx, req)
		if err != nil {
			return Failure(err), app.RelationView{}, nil
		}
		return nil, res, nil
	default:
		return invalid("unknown action " + in.Action)
	}
}

// handleWriteOrigin is the write_origin tool. The action-to-fields shape
// and the confirmation guard are transport concerns; the journal and the
// revision guard are the shared service's.
func (s *Server) handleWriteOrigin(ctx context.Context, _ *mcpsdk.CallToolRequest, in writeOriginInput) (*mcpsdk.CallToolResult, app.OriginView, error) {
	base := app.MutationBase{
		RequestID:        in.RequestID,
		Actor:            in.Actor.toApp(),
		ExpectedRevision: in.ExpectedRevision,
	}
	invalid := func(message string) (*mcpsdk.CallToolResult, app.OriginView, error) {
		return Failure(app.NewAppError(app.ErrInvalidArgument, message)), app.OriginView{}, nil
	}
	switch in.Action {
	case RelationPropose:
		if in.EditionID == nil || in.Kind == nil || in.Provenance == nil {
			return invalid("propose requires edition_id, kind and provenance")
		}
		state := domain.ClaimProposed
		req := app.RecordOriginRequest{
			MutationBase: base,
			EditionID:    *in.EditionID,
			Kind:         domain.OriginKind(*in.Kind),
			State:        state,
			Provenance:   *in.Provenance,
			ReplacesID:   in.ReplacesID,
		}
		res, err := s.Service().RecordOrigin(ctx, req)
		if err != nil {
			return Failure(err), app.OriginView{}, nil
		}
		return nil, res, nil
	case RelationConfirm:
		if in.EditionID == nil || in.ClaimID == nil {
			return invalid("confirm requires edition_id and claim_id")
		}
		if err := confirmationGuard(struct {
			Confirmation    bool
			UserInstruction *string
		}{in.Confirmation, in.UserInstruction}); err != nil {
			return Failure(err), app.OriginView{}, nil
		}
		req := app.ConfirmOriginRequest{
			MutationBase: base,
			EditionID:    *in.EditionID,
			ClaimID:      *in.ClaimID,
		}
		res, err := s.Service().ConfirmOrigin(ctx, req)
		if err != nil {
			return Failure(err), app.OriginView{}, nil
		}
		return nil, res, nil
	case RelationRevoke:
		if in.EditionID == nil || in.ClaimID == nil {
			return invalid("revoke requires edition_id and claim_id")
		}
		req := app.RevokeOriginRequest{
			MutationBase: base,
			EditionID:    *in.EditionID,
			ClaimID:      *in.ClaimID,
		}
		res, err := s.Service().RevokeOrigin(ctx, req)
		if err != nil {
			return Failure(err), app.OriginView{}, nil
		}
		return nil, res, nil
	default:
		return invalid("unknown action " + in.Action)
	}
}
