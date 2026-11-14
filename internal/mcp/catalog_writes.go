package mcp

import (
	"context"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Catalog write tool names.
const (
	ToolResolveAddition = "resolve_addition"
	ToolCreateEntry     = "create_entry"
	ToolUpdateCatalog   = "update_catalog"
)

// update_catalog actions.
const (
	ActionAddEdition        = "add_edition"
	ActionAddTitle          = "add_title"
	ActionRetireTitle       = "retire_title"
	ActionSetPreferredTitle = "set_preferred_title"
	ActionUpdateMetadata    = "update_metadata"
	ActionArchive           = "archive"
	ActionUnarchive         = "unarchive"
)

func titleSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"text", "kind", "provenance"},
		"properties": map[string]any{
			"text":       map[string]any{"type": "string", "minLength": 1},
			"kind":       map[string]any{"type": "string", "enum": []any{string(domain.TitleOriginal), string(domain.TitleLocalized), string(domain.TitleRomanized), string(domain.TitleAlias)}},
			"language":   map[string]any{"type": []any{"null", "string"}},
			"preferred":  map[string]any{"type": "boolean"},
			"provenance": map[string]any{"type": "string", "minLength": 1},
		},
	}
}

func referenceSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"description":          "A sourced external identity: exactly one of external_id (with a provider) or url.",
		"properties": map[string]any{
			"provider":    map[string]any{"type": []any{"null", "string"}},
			"external_id": map[string]any{"type": []any{"null", "string"}},
			"url":         map[string]any{"type": []any{"null", "string"}},
		},
	}
}

func actorSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"principal", "origin"},
		"properties": map[string]any{
			"principal": map[string]any{"type": "string", "minLength": 1},
			"origin":    map[string]any{"type": "string", "enum": []any{string(domain.OriginUser), string(domain.OriginAgent), string(domain.OriginImport)}},
			"identity":  map[string]any{"type": []any{"null", "string"}},
		},
	}
}

func kindSchema() map[string]any {
	return map[string]any{
		"type": []any{"null", "string"},
		"enum": []any{
			nil, string(domain.KindWebNovel), string(domain.KindLightNovel), string(domain.KindBook), string(domain.KindManga), string(domain.KindManhwa), string(domain.KindManhua), string(domain.KindAnime), string(domain.KindMovie), string(domain.KindTV), string(domain.KindGame), string(domain.KindVisualNovel), string(domain.KindOther), string(domain.KindUnknown),
		},
	}
}

var resolveAdditionSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"titles": map[string]any{
			"type":     "array",
			"minItems": 1,
			"items":    titleSchema(),
		},
		"references": map[string]any{
			"type":  "array",
			"items": referenceSchema(),
		},
		"kind":        kindSchema(),
		"language":    map[string]any{"type": []any{"null", "string"}},
		"other_label": map[string]any{"type": []any{"null", "string"}},
	},
}

var createEntrySchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor"},
	"properties": map[string]any{
		"request_id":      map[string]any{"type": "string", "minLength": 1},
		"actor":           actorSchema(),
		"titles":          map[string]any{"type": "array", "items": titleSchema()},
		"references":      map[string]any{"type": "array", "items": referenceSchema()},
		"work_id":         map[string]any{"type": []any{"null", "string"}},
		"kind":            kindSchema(),
		"language":        map[string]any{"type": []any{"null", "string"}},
		"other_label":     map[string]any{"type": []any{"null", "string"}},
		"choice": map[string]any{
			"type": []any{"null", "string"},
			"enum": []any{nil, string(app.ChoiceUseExisting), string(app.ChoiceAddEdition), string(app.ChoiceCreateDistinct)},
		},
		"candidate_token": map[string]any{"type": []any{"null", "string"}},
	},
}

var updateCatalogSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "action"},
	"properties": map[string]any{
		"request_id": map[string]any{"type": "string", "minLength": 1},
		"actor":      actorSchema(),
		"action": map[string]any{
			"type": "string",
			"enum": []any{ActionAddEdition, ActionAddTitle, ActionRetireTitle, ActionSetPreferredTitle, ActionUpdateMetadata, ActionArchive, ActionUnarchive},
		},
		"expected_revision":  map[string]any{"type": []any{"null", "integer"}},
		"work_id":            map[string]any{"type": []any{"null", "string"}},
		"edition_id":         map[string]any{"type": []any{"null", "string"}},
		"target_scope":       map[string]any{"type": []any{"null", "string"}, "enum": []any{nil, "work", "edition"}},
		"add_titles":         map[string]any{"type": "array", "items": titleSchema()},
		"retire_title_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"preferred_title_id": map[string]any{"type": []any{"null", "string"}},
		"kind":               kindSchema(),
		"language":           map[string]any{"type": []any{"null", "string"}},
		"other_label":        map[string]any{"type": []any{"null", "string"}},
		"titles":             map[string]any{"type": "array", "items": titleSchema()},
		"references":         map[string]any{"type": "array", "items": referenceSchema()},
	},
}

type titleInput struct {
	Text       string  `json:"text"`
	Kind       string  `json:"kind"`
	Language   *string `json:"language"`
	Preferred  bool    `json:"preferred"`
	Provenance string  `json:"provenance"`
}

func (t titleInput) toStore() store.TitleInput {
	return store.TitleInput{
		Text:       t.Text,
		Kind:       domain.TitleKind(t.Kind),
		Language:   t.Language,
		Preferred:  t.Preferred,
		Provenance: t.Provenance,
	}
}

func titlesToStore(values []titleInput) []store.TitleInput {
	out := make([]store.TitleInput, 0, len(values))
	for _, v := range values {
		out = append(out, v.toStore())
	}
	return out
}

type referenceInput struct {
	Provider   *string `json:"provider"`
	ExternalID *string `json:"external_id"`
	URL        *string `json:"url"`
}

func (r referenceInput) toStore() store.ReferenceInput {
	return store.ReferenceInput{Provider: r.Provider, ExternalID: r.ExternalID, URL: r.URL}
}

func referencesToStore(values []referenceInput) []store.ReferenceInput {
	out := make([]store.ReferenceInput, 0, len(values))
	for _, v := range values {
		out = append(out, v.toStore())
	}
	return out
}

type actorInput struct {
	Principal string  `json:"principal"`
	Origin    string  `json:"origin"`
	Identity  *string `json:"identity"`
}

func (a actorInput) toApp() app.Actor {
	actor := app.Actor{Principal: a.Principal, Origin: domain.ActorOrigin(a.Origin)}
	if a.Identity != nil {
		actor.Identity = *a.Identity
	}
	return actor
}

type resolveAdditionInput struct {
	Titles     []titleInput     `json:"titles"`
	References []referenceInput `json:"references"`
	Kind       *string          `json:"kind"`
	Language   *string          `json:"language"`
	OtherLabel *string          `json:"other_label"`
}

type createEntryInput struct {
	RequestID      string           `json:"request_id"`
	Actor          actorInput       `json:"actor"`
	Titles         []titleInput     `json:"titles"`
	References     []referenceInput `json:"references"`
	WorkID         *string          `json:"work_id"`
	Kind           *string          `json:"kind"`
	Language       *string          `json:"language"`
	OtherLabel     *string          `json:"other_label"`
	Choice         *string          `json:"choice"`
	CandidateToken *string          `json:"candidate_token"`
}

type updateCatalogInput struct {
	RequestID        string           `json:"request_id"`
	Actor            actorInput       `json:"actor"`
	Action           string           `json:"action"`
	ExpectedRevision *int64           `json:"expected_revision"`
	WorkID           *string          `json:"work_id"`
	EditionID        *string          `json:"edition_id"`
	TargetScope      *string          `json:"target_scope"`
	AddTitles        []titleInput     `json:"add_titles"`
	RetireTitleIDs   []string         `json:"retire_title_ids"`
	PreferredTitleID *string          `json:"preferred_title_id"`
	Kind             *string          `json:"kind"`
	Language         *string          `json:"language"`
	OtherLabel       *string          `json:"other_label"`
	Titles           []titleInput     `json:"titles"`
	References       []referenceInput `json:"references"`
}

// RegisterCatalogWriteTools installs the catalog write tools. The identity
// decision never bypasses the shared service: an unresolved candidate
// creates nothing until an explicit choice with its short-lived token is
// presented, and a matching URL or external ID resolves to the existing
// entry instead of cloning it.
func RegisterCatalogWriteTools(s *Server) {
	registeredTools = append(registeredTools, ToolResolveAddition, ToolCreateEntry, ToolUpdateCatalog)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolResolveAddition,
		Description: "Resolve a library addition against the existing catalog before creating anything. Returns the outcome: ready (no match), existing (a known identity matched; the work ID is returned), or candidates (exact title matches exist; each candidate carries evidence and a short-lived token). Present the candidates to the user; the choice is theirs, not an inference.",
		InputSchema: resolveAdditionSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleResolveAddition)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolCreateEntry,
		Description: "Create a library entry, idempotently. A title, a known identity or an explicit work is required. When exact title candidates exist, resolve_addition must be called first and the explicit choice (use_existing, add_edition or create_distinct) plus its candidate token passed here; without a choice an exact match is a duplicate_candidate error and nothing is created. A matching URL or external ID never clones: it resolves to the existing entry. The same request ID with the same payload replays the stored result; with a different payload it is a conflict.",
		InputSchema: createEntrySchema,
		Annotations: writeAnnotations(),
	}, s.handleCreateEntry)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolUpdateCatalog,
		Description: "Apply one catalog correction. Actions: add_edition (a new edition line on an existing work via the canonical addition path, work_id plus a title or a known identity required), add_title, retire_title and set_preferred_title (target_scope work or edition with the matching id, plus add_titles, retire_title_ids or preferred_title_id), update_metadata (edition_id plus at least one of kind, language, other_label) and archive/unarchive (target_scope work or edition with the matching id). Existing-entity actions require the expected_revision read from the current view; a stale revision is a conflict and the prior state stays intact.",
		InputSchema: updateCatalogSchema,
		Annotations: writeAnnotations(),
	}, s.handleUpdateCatalog)
}

// writeAnnotations marks the write tools: they modify the library, a
// repeated call with the same idempotency key replays, and the domain is
// the closed personal library.
func writeAnnotations() *mcpsdk.ToolAnnotations {
	f := false
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    false,
		IdempotentHint:  true,
		DestructiveHint: &f,
		OpenWorldHint:   &f,
	}
}

// handleResolveAddition is the resolve_addition tool. It is a decision,
// not a mutation: the candidate list and its short-lived token travel to
// the caller, and nothing is created here.
func (s *Server) handleResolveAddition(ctx context.Context, _ *mcpsdk.CallToolRequest, in resolveAdditionInput) (*mcpsdk.CallToolResult, app.ResolveResult, error) {
	req := app.AdditionRequest{
		Titles:     titlesToStore(in.Titles),
		References: referencesToStore(in.References),
		Kind:       kindPtr(in.Kind),
		Language:   in.Language,
		OtherLabel: in.OtherLabel,
	}
	res, err := s.Service().ResolveAddition(ctx, req)
	if err != nil {
		return Failure(err), app.ResolveResult{}, nil
	}
	return nil, res, nil
}

// handleCreateEntry is the create_entry tool.
func (s *Server) handleCreateEntry(ctx context.Context, _ *mcpsdk.CallToolRequest, in createEntryInput) (*mcpsdk.CallToolResult, app.AdditionResult, error) {
	req := app.AdditionRequest{
		MutationBase: app.MutationBase{
			RequestID: in.RequestID,
			Actor:     in.Actor.toApp(),
		},
		Titles:         titlesToStore(in.Titles),
		References:     referencesToStore(in.References),
		WorkID:         in.WorkID,
		Kind:           kindPtr(in.Kind),
		Language:       in.Language,
		OtherLabel:     in.OtherLabel,
		Choice:         choicePtr(in.Choice),
		CandidateToken: in.CandidateToken,
	}
	res, err := s.Service().CreateLibraryEntry(ctx, req)
	if err != nil {
		return Failure(err), app.AdditionResult{}, nil
	}
	return nil, res, nil
}

// handleUpdateCatalog is the update_catalog tool. The action-to-fields
// shape is checked here (transport concern); every domain rule — the
// revision guard, the journal, the identity checks — is the shared
// service's.
func (s *Server) handleUpdateCatalog(ctx context.Context, _ *mcpsdk.CallToolRequest, in updateCatalogInput) (*mcpsdk.CallToolResult, any, error) {
	base := app.MutationBase{
		RequestID:        in.RequestID,
		Actor:            in.Actor.toApp(),
		ExpectedRevision: in.ExpectedRevision,
	}
	invalid := func(message string) (*mcpsdk.CallToolResult, any, error) {
		return Failure(app.NewAppError(app.ErrInvalidArgument, message)), nil, nil
	}
	switch in.Action {
	case ActionAddEdition:
		if in.WorkID == nil {
			return invalid("add_edition requires work_id")
		}
		if len(in.Titles) == 0 && len(in.References) == 0 {
			return invalid("add_edition requires a title or a known identity")
		}
		choice := app.ChoiceAddEdition
		req := app.AdditionRequest{
			MutationBase: base,
			Titles:       titlesToStore(in.Titles),
			References:   referencesToStore(in.References),
			WorkID:       in.WorkID,
			Kind:         kindPtr(in.Kind),
			Language:     in.Language,
			OtherLabel:   in.OtherLabel,
			Choice:       &choice,
		}
		res, err := s.Service().CreateLibraryEntry(ctx, req)
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, res, nil
	case ActionAddTitle, ActionRetireTitle, ActionSetPreferredTitle:
		req, ok := titleMutation(base, in)
		if !ok {
			return invalid(titleMutationUsage(in.Action))
		}
		var view any
		var err error
		if *in.TargetScope == "work" {
			view, err = s.Service().CorrectWorkTitles(ctx, req)
		} else {
			view, err = s.Service().CorrectEditionTitles(ctx, req)
		}
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, view, nil
	case ActionUpdateMetadata:
		if in.EditionID == nil {
			return invalid("update_metadata requires edition_id")
		}
		if in.Kind == nil && in.Language == nil && in.OtherLabel == nil {
			return invalid("update_metadata requires at least one of kind, language, other_label")
		}
		req := app.UpdateEditionRequest{
			MutationBase: base,
			EditionID:    *in.EditionID,
			Kind:         kindPtr(in.Kind),
			Language:     in.Language,
			OtherLabel:   in.OtherLabel,
		}
		view, err := s.Service().UpdateEdition(ctx, req)
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, view, nil
	case ActionArchive, ActionUnarchive:
		targetID, ok := scopedTarget(in)
		if !ok {
			return invalid("archive and unarchive require target_scope with the matching " + scopeTargetName(in) + "_id")
		}
		archived := in.Action == ActionArchive
		var view any
		var err error
		if *in.TargetScope == "work" {
			view, err = s.Service().ArchiveWork(ctx, base, targetID, archived)
		} else {
			view, err = s.Service().ArchiveEdition(ctx, base, targetID, archived)
		}
		if err != nil {
			return Failure(err), nil, nil
		}
		return nil, view, nil
	default:
		return invalid("unknown action " + in.Action)
	}
}

// titleMutation builds the title correction request for the title actions:
// the target scope selects the id field and the target kind.
func titleMutation(base app.MutationBase, in updateCatalogInput) (app.TitleMutationRequest, bool) {
	targetID, ok := scopedTarget(in)
	if !ok {
		return app.TitleMutationRequest{}, false
	}
	req := app.TitleMutationRequest{
		MutationBase: base,
		TargetID:     targetID,
		Add:          titlesToStore(in.AddTitles),
		Retire:       in.RetireTitleIDs,
		Preferred:    in.PreferredTitleID,
	}
	switch in.Action {
	case ActionAddTitle:
		if len(req.Add) == 0 {
			return app.TitleMutationRequest{}, false
		}
	case ActionRetireTitle:
		if len(req.Retire) == 0 {
			return app.TitleMutationRequest{}, false
		}
	case ActionSetPreferredTitle:
		if req.Preferred == nil {
			return app.TitleMutationRequest{}, false
		}
	}
	return req, true
}

// scopedTarget resolves the id for a scope-targeted action: scope work
// takes work_id, scope edition takes edition_id, and the other id must be
// absent so a correction cannot land on the wrong entity.
func scopedTarget(in updateCatalogInput) (string, bool) {
	if in.TargetScope == nil {
		return "", false
	}
	switch *in.TargetScope {
	case "work":
		if in.WorkID == nil || in.EditionID != nil {
			return "", false
		}
		return *in.WorkID, true
	case "edition":
		if in.EditionID == nil || in.WorkID != nil {
			return "", false
		}
		return *in.EditionID, true
	default:
		return "", false
	}
}

func scopeTargetName(in updateCatalogInput) string {
	if in.TargetScope != nil && *in.TargetScope == "edition" {
		return "edition"
	}
	return "work"
}

func titleMutationUsage(action string) string {
	return action + " requires target_scope (work or edition) with the matching id"
}

func choicePtr(v *string) *app.AdditionChoice {
	if v == nil {
		return nil
	}
	c := app.AdditionChoice(*v)
	return &c
}

func kindPtr(v *string) *domain.EditionKind {
	if v == nil {
		return nil
	}
	k := domain.EditionKind(*v)
	return &k
}
