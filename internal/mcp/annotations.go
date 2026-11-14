package mcp

import (
	"context"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Personal annotation tool names.
const (
	ToolSetIntention   = "set_intention"
	ToolSetTag         = "set_tag"
	ToolWriteNote      = "write_note"
	ToolRecordRating   = "record_rating"
	ToolWriteReference = "write_reference"
)

// write_note actions.
const (
	NoteAdd     = "add"
	NoteRevise  = "revise"
	NoteRetract = "retract"
)

// write_reference actions.
const (
	ReferenceAdd    = "add"
	ReferenceRetire = "retire"
)

func intentionValueSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{
			string(domain.IntentionNone), string(domain.IntentionInterested),
			string(domain.IntentionSaved), string(domain.IntentionPlanned),
			string(domain.IntentionNotInterested),
		},
	}
}

func referenceRoleSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{
			string(domain.RolePublication), string(domain.RoleMetadata),
			string(domain.RoleTranslation), string(domain.RoleReleaseWatcher),
			string(domain.RoleTranslationProject), string(domain.RoleContent),
		},
	}
}

var setIntentionSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "edition_id", "value", "expected_revision"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"edition_id":        map[string]any{"type": "string", "minLength": 1},
		"value":             intentionValueSchema(),
		"reason":            map[string]any{"type": []any{"null", "string"}},
		"observed_at":       map[string]any{"type": []any{"null", "string"}, "format": "date-time"},
		"supersedes_id":     map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var setTagSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "target_scope", "target_id", "tag"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"target_scope":      map[string]any{"type": "string", "enum": []any{"work", "edition"}},
		"target_id":         map[string]any{"type": "string", "minLength": 1},
		"tag":               map[string]any{"type": "string", "minLength": 1},
		"detach":            map[string]any{"type": "boolean"},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var writeNoteSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "action"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"action":            map[string]any{"type": "string", "enum": []any{NoteAdd, NoteRevise, NoteRetract}},
		"target_scope":      map[string]any{"type": []any{"null", "string"}, "enum": []any{nil, "work", "edition"}},
		"target_id":         map[string]any{"type": []any{"null", "string"}},
		"note_id":           map[string]any{"type": []any{"null", "string"}},
		"text":              map[string]any{"type": []any{"null", "string"}},
		"spoiler":           map[string]any{"type": "boolean"},
		"quote":             map[string]any{"type": []any{"null", "string"}},
		"location":          map[string]any{"type": []any{"null", "string"}},
		"reason":            map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var recordRatingSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "edition_id", "expected_revision"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"edition_id":        map[string]any{"type": "string", "minLength": 1},
		"value":             map[string]any{"type": []any{"null", "integer"}, "minimum": 1, "maximum": 10},
		"comment":           map[string]any{"type": []any{"null", "string"}},
		"observed_at":       map[string]any{"type": []any{"null", "string"}, "format": "date-time"},
		"supersedes_id":     map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var writeReferenceSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "action", "edition_id"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"action":            map[string]any{"type": "string", "enum": []any{ReferenceAdd, ReferenceRetire}},
		"edition_id":        map[string]any{"type": "string", "minLength": 1},
		"provider":          map[string]any{"type": []any{"null", "string"}},
		"external_id":       map[string]any{"type": []any{"null", "string"}},
		"url":               map[string]any{"type": []any{"null", "string"}},
		"role":              referenceRoleSchema(),
		"authoritative":     map[string]any{"type": "boolean"},
		"language":          map[string]any{"type": []any{"null", "string"}},
		"provenance":        map[string]any{"type": []any{"null", "string"}},
		"reference_id":      map[string]any{"type": []any{"null", "string"}},
		"reason":            map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

type setIntentionInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	EditionID        string     `json:"edition_id"`
	Value            string     `json:"value"`
	Reason           *string    `json:"reason"`
	ObservedAt       *string    `json:"observed_at"`
	SupersedesID     *string    `json:"supersedes_id"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type setTagInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	TargetScope      string     `json:"target_scope"`
	TargetID         string     `json:"target_id"`
	Tag              string     `json:"tag"`
	Detach           bool       `json:"detach"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type writeNoteInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	Action           string     `json:"action"`
	TargetScope      *string    `json:"target_scope"`
	TargetID         *string    `json:"target_id"`
	NoteID           *string    `json:"note_id"`
	Text             *string    `json:"text"`
	Spoiler          bool       `json:"spoiler"`
	Quote            *string    `json:"quote"`
	Location         *string    `json:"location"`
	Reason           *string    `json:"reason"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type recordRatingInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	EditionID        string     `json:"edition_id"`
	Value            *int       `json:"value"`
	Comment          *string    `json:"comment"`
	ObservedAt       *string    `json:"observed_at"`
	SupersedesID     *string    `json:"supersedes_id"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type writeReferenceInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	Action           string     `json:"action"`
	EditionID        string     `json:"edition_id"`
	Provider         *string    `json:"provider"`
	ExternalID       *string    `json:"external_id"`
	URL              *string    `json:"url"`
	Role             string     `json:"role"`
	Authoritative    bool       `json:"authoritative"`
	Language         *string    `json:"language"`
	Provenance       *string    `json:"provenance"`
	ReferenceID      *string    `json:"reference_id"`
	Reason           *string    `json:"reason"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

func init() {
	trackTools(ToolSetIntention, ToolSetTag, ToolWriteNote, ToolRecordRating, ToolWriteReference)
}

// RegisterAnnotationTools installs the personal annotation tools. Each
// action routes to the shared service; the transport validates only the
// schema shape and the action-to-fields mapping, and defers every domain
// rule (value bounds, the revision guard, the journal) to the service.
func RegisterAnnotationTools(s *Server) {
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolSetIntention,
		Description: "Set the personal reading intention of an edition: none, interested, saved, planned or not_interested (none clears the state). A supersedes_id correction retracts the named row, which stays in history. expected_revision comes from the current edition view; a stale revision is a conflict. The same request ID with the same payload replays the stored result.",
		InputSchema: setIntentionSchema,
		Annotations: writeAnnotations(),
	}, s.handleSetIntention)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolSetTag,
		Description: "Attach or detach a normalized tag on a work or an edition (target_scope work or edition with the matching target_id). detach true removes the tag instead of adding it. expected_revision comes from the current view; a stale revision is a conflict. The same request ID with the same payload replays the stored result.",
		InputSchema: setTagSchema,
		Annotations: writeAnnotations(),
	}, s.handleSetTag)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolWriteNote,
		Description: "Write one plain-text note on a work or an edition. Actions: add (target_scope work or edition with the matching target_id and text), revise (note_id plus the new text, spoiler, quote, location) and retract (note_id plus a reason). Notes keep immutable revisions; a retraction or revision never deletes the prior rows. expected_revision comes from the current view; a stale revision is a conflict.",
		InputSchema: writeNoteSchema,
		Annotations: writeAnnotations(),
	}, s.handleWriteNote)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolRecordRating,
		Description: "Record a 1-10 rating for an edition, or pass a null value to clear it explicitly. A supersedes_id correction retracts the named row, which stays in history with its retained changes. expected_revision comes from the current edition view; a stale revision is a conflict. The same request ID with the same payload replays the stored result.",
		InputSchema: recordRatingSchema,
		Annotations: writeAnnotations(),
	}, s.handleRecordRating)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolWriteReference,
		Description: "Attach or retire a sourced external reference on an edition. Actions: add (edition_id plus a role, provenance and exactly one of provider+external_id or url) and retire (edition_id plus reference_id and a reason; identity-safe, the row stays in history). expected_revision comes from the current edition view; a stale revision is a conflict.",
		InputSchema: writeReferenceSchema,
		Annotations: writeAnnotations(),
	}, s.handleWriteReference)
}

// handleSetIntention is the set_intention tool.
func (s *Server) handleSetIntention(ctx context.Context, _ *mcpsdk.CallToolRequest, in setIntentionInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	var observedAt *time.Time
	if in.ObservedAt != nil && *in.ObservedAt != "" {
		ts, err := parseInstant(in.ObservedAt)
		if err != nil {
			return Failure(err), app.EditionView{}, nil
		}
		observedAt = &ts
	}
	req := app.SetIntentionRequest{
		MutationBase: app.MutationBase{
			RequestID:        in.RequestID,
			Actor:            in.Actor.toApp(),
			ExpectedRevision: in.ExpectedRevision,
		},
		EditionID:    in.EditionID,
		Value:        in.Value,
		Reason:       in.Reason,
		ObservedAt:   observedAt,
		SupersedesID: in.SupersedesID,
	}
	res, err := s.Service().SetIntention(ctx, req)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	return nil, res, nil
}

// handleSetTag is the set_tag tool. The target scope selects the shared
// service method; the tag normalization and the revision guard are the
// service's.
func (s *Server) handleSetTag(ctx context.Context, _ *mcpsdk.CallToolRequest, in setTagInput) (*mcpsdk.CallToolResult, any, error) {
	req := app.TagRequest{
		MutationBase: app.MutationBase{
			RequestID:        in.RequestID,
			Actor:            in.Actor.toApp(),
			ExpectedRevision: in.ExpectedRevision,
		},
		TargetID: in.TargetID,
		Tag:      in.Tag,
		Detach:   in.Detach,
	}
	var (
		view any
		err  error
	)
	switch in.TargetScope {
	case "work":
		view, err = s.Service().SetWorkTag(ctx, req)
	case "edition":
		view, err = s.Service().SetEditionTag(ctx, req)
	default:
		return Failure(app.NewAppError(app.ErrInvalidArgument, "target_scope must be work or edition")), nil, nil
	}
	if err != nil {
		return Failure(err), nil, nil
	}
	return nil, view, nil
}

// handleWriteNote is the write_note tool. The action-to-fields shape is
// checked here (transport concern); the revision chain and the guard are
// the shared service's.
func (s *Server) handleWriteNote(ctx context.Context, _ *mcpsdk.CallToolRequest, in writeNoteInput) (*mcpsdk.CallToolResult, any, error) {
	base := app.MutationBase{
		RequestID:        in.RequestID,
		Actor:            in.Actor.toApp(),
		ExpectedRevision: in.ExpectedRevision,
	}
	invalid := func(message string) (*mcpsdk.CallToolResult, any, error) {
		return Failure(app.NewAppError(app.ErrInvalidArgument, message)), nil, nil
	}
	switch in.Action {
	case NoteAdd:
		if in.TargetScope == nil || in.TargetID == nil {
			return invalid("add requires target_scope (work or edition) with the matching target_id")
		}
		if in.Text == nil || *in.Text == "" {
			return invalid("add requires text")
		}
		req := app.AddNoteRequest{
			MutationBase: base,
			TargetID:     *in.TargetID,
			Text:         *in.Text,
			Spoiler:      in.Spoiler,
			Quote:        in.Quote,
			Location:     in.Location,
		}
		var (
			view any
			err  error
		)
		switch *in.TargetScope {
		case "work":
			view, err = s.Service().AddWorkNote(ctx, req)
		case "edition":
			view, err = s.Service().AddEditionNote(ctx, req)
		default:
			return invalid("target_scope must be work or edition")
		}
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, view, nil
	case NoteRevise:
		if in.NoteID == nil || in.Text == nil {
			return invalid("revise requires note_id and text")
		}
		req := app.ReviseNoteRequest{
			MutationBase: base,
			NoteID:       *in.NoteID,
			Text:         *in.Text,
			Spoiler:      in.Spoiler,
			Quote:        in.Quote,
			Location:     in.Location,
		}
		view, err := s.Service().ReviseNote(ctx, req)
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, view, nil
	case NoteRetract:
		if in.NoteID == nil {
			return invalid("retract requires note_id")
		}
		req := app.RetractNoteRequest{
			MutationBase: base,
			NoteID:       *in.NoteID,
			Reason:       in.Reason,
		}
		view, err := s.Service().RetractNote(ctx, req)
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, view, nil
	default:
		return invalid("unknown action " + in.Action)
	}
}

// handleRecordRating is the record_rating tool.
func (s *Server) handleRecordRating(ctx context.Context, _ *mcpsdk.CallToolRequest, in recordRatingInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	var observedAt *time.Time
	if in.ObservedAt != nil && *in.ObservedAt != "" {
		ts, err := parseInstant(in.ObservedAt)
		if err != nil {
			return Failure(err), app.EditionView{}, nil
		}
		observedAt = &ts
	}
	req := app.SetRatingRequest{
		MutationBase: app.MutationBase{
			RequestID:        in.RequestID,
			Actor:            in.Actor.toApp(),
			ExpectedRevision: in.ExpectedRevision,
		},
		EditionID:    in.EditionID,
		Value:        in.Value,
		Comment:      in.Comment,
		ObservedAt:   observedAt,
		SupersedesID: in.SupersedesID,
	}
	res, err := s.Service().SetRating(ctx, req)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	return nil, res, nil
}

// handleWriteReference is the write_reference tool. The action-to-fields
// shape is checked here; the identity reservation and the guard are the
// shared service's.
func (s *Server) handleWriteReference(ctx context.Context, _ *mcpsdk.CallToolRequest, in writeReferenceInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	base := app.MutationBase{
		RequestID:        in.RequestID,
		Actor:            in.Actor.toApp(),
		ExpectedRevision: in.ExpectedRevision,
	}
	switch in.Action {
	case ReferenceAdd:
		if in.Role == "" {
			return Failure(app.NewAppError(app.ErrInvalidArgument, "add requires a role")), app.EditionView{}, nil
		}
		req := app.AddReferenceRequest{
			MutationBase:  base,
			EditionID:     in.EditionID,
			Provider:      derefString(in.Provider),
			ExternalID:    in.ExternalID,
			URL:           in.URL,
			Role:          domain.ReferenceRole(in.Role),
			Authoritative: in.Authoritative,
			Language:      in.Language,
			Provenance:    derefString(in.Provenance),
		}
		res, err := s.Service().AddReference(ctx, req)
		if err != nil {
			return Failure(err), app.EditionView{}, nil
		}
		return nil, res, nil
	case ReferenceRetire:
		if in.ReferenceID == nil {
			return Failure(app.NewAppError(app.ErrInvalidArgument, "retire requires reference_id")), app.EditionView{}, nil
		}
		req := app.RetireReferenceRequest{
			MutationBase: base,
			EditionID:    in.EditionID,
			ReferenceID:  *in.ReferenceID,
			Reason:       in.Reason,
		}
		res, err := s.Service().RetireReference(ctx, req)
		if err != nil {
			return Failure(err), app.EditionView{}, nil
		}
		return nil, res, nil
	default:
		return Failure(app.NewAppError(app.ErrInvalidArgument, "unknown action "+in.Action)), app.EditionView{}, nil
	}
}

// derefString returns the pointed-to string or an empty string.
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
