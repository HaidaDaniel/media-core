package mcp

import (
	"context"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Catalog read tool names.
const (
	ToolSearchLibrary = "search_library"
	ToolGetWork       = "get_work"
	ToolGetEdition    = "get_edition"
)

// readOnlyAnnotations marks the catalog tools: they never modify the
// library, repeating them has no additional effect, and their domain is
// the closed personal library.
func readOnlyAnnotations() *mcpsdk.ToolAnnotations {
	f := false
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint:    true,
		IdempotentHint:  true,
		DestructiveHint: &f,
		OpenWorldHint:   &f,
	}
}

// stringArray builds a bounded array-of-string schema; enum constrains the
// items to the defined v1 values.
func stringArray(description string, enum ...string) map[string]any {
	items := map[string]any{"type": "string"}
	if len(enum) > 0 {
		values := make([]any, len(enum))
		for i, v := range enum {
			values[i] = v
		}
		items["enum"] = values
	}
	return map[string]any{
		"type":        "array",
		"items":       items,
		"uniqueItems": true,
		"description": description,
	}
}

// The schemas are hand-written 2020-12 drafts, not inferred: they are the
// bounded contract of the tool surface, so unknown fields are rejected at
// the schema level (additionalProperties false) and every dimension is
// constrained to the values the shared service defines. The service still
// enforces every bound, value and cursor rule after decoding; the schema
// only shapes the request.

var searchLibrarySchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"query": map[string]any{
			"type":        "string",
			"maxLength":   app.MaxLibraryQueryRunes,
			"description": "Search text matched against normalized titles (JP/RU/romaji) and metadata; empty lists the whole library.",
		},
		"kinds": stringArray("Edition line kinds to include.",
			string(domain.KindWebNovel), string(domain.KindLightNovel), string(domain.KindBook), string(domain.KindManga), string(domain.KindManhwa), string(domain.KindManhua), string(domain.KindAnime), string(domain.KindMovie), string(domain.KindTV), string(domain.KindGame), string(domain.KindVisualNovel), string(domain.KindOther), string(domain.KindUnknown)),
		"publications": stringArray("Publication dimension values.",
			string(domain.PublicationUnknown), string(domain.PublicationOngoing), string(domain.PublicationCompleted), string(domain.PublicationHiatus), string(domain.PublicationCancelled)),
		"consumptions": stringArray("Consumption dimension values.",
			string(domain.ConsumptionNotStarted), string(domain.ConsumptionInProgress), string(domain.ConsumptionPaused), string(domain.ConsumptionCompleted), string(domain.ConsumptionDropped)),
		"unknown_consumption": map[string]any{
			"type":        "boolean",
			"description": "Select editions whose current effective consumption is unknown.",
		},
		"intentions": stringArray("Intention dimension values.",
			string(domain.IntentionNone), string(domain.IntentionInterested), string(domain.IntentionSaved), string(domain.IntentionPlanned), string(domain.IntentionNotInterested)),
		"tags": stringArray("Tag names; normalized before matching, satisfied by a work tag or the matching edition's own tag."),
		"min_rating": map[string]any{
			"type":        "integer",
			"minimum":     1,
			"maximum":     10,
			"description": "The current rating of the matching edition must be at least this value; an absent or cleared rating never matches.",
		},
		"sort": map[string]any{
			"type": "string",
			"enum": []any{string(app.SortTitle), string(app.SortCreatedAt), string(app.SortUpdatedAt), string(app.SortRating)},
		},
		"limit": map[string]any{
			"type":    "integer",
			"minimum": 1,
			"maximum": app.MaxLibraryLimit,
		},
		"cursor": map[string]any{
			"type":        "string",
			"description": "Opaque seek cursor from a previous page of the same request; empty starts at the beginning.",
		},
	},
}

var getWorkSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"work_id"},
	"properties": map[string]any{
		"work_id": map[string]any{
			"type":        "string",
			"minLength":   1,
			"description": "The work ID.",
		},
	},
}

var getEditionSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"edition_id"},
	"properties": map[string]any{
		"edition_id": map[string]any{
			"type":        "string",
			"minLength":   1,
			"description": "The edition ID.",
		},
	},
}

type searchLibraryInput struct {
	Query              string   `json:"query"`
	Kinds              []string `json:"kinds"`
	Publications       []string `json:"publications"`
	Consumptions       []string `json:"consumptions"`
	UnknownConsumption bool     `json:"unknown_consumption"`
	Intentions         []string `json:"intentions"`
	Tags               []string `json:"tags"`
	MinRating          *int     `json:"min_rating"`
	Sort               string   `json:"sort"`
	Limit              int      `json:"limit"`
	Cursor             string   `json:"cursor"`
}

type getWorkInput struct {
	WorkID string `json:"work_id"`
}

type getEditionInput struct {
	EditionID string `json:"edition_id"`
}

// RegisterLibraryTools installs the read-only catalog tools. The handlers
// call the shared service and render outcomes through the typed
// translation, so the transport owns no catalog rule: every bound, value
// and cursor rule is the one the shared service already enforces for
// HTTP and CLI.
func RegisterLibraryTools(s *Server) {
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolSearchLibrary,
		Description: "Search the personal media library as work cards. The query matches normalized titles (Japanese, Russian, romaji) and metadata; every filter dimension is optional and combinable. Returns the shared catalog list DTO with the matching edition IDs per card and an opaque cursor for the next page of the same request.",
		InputSchema: searchLibrarySchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleSearchLibrary)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolGetWork,
		Description: "Get the detail view of one work: identity, active sourced titles and edition summaries, in the shared work DTO.",
		InputSchema: getWorkSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleGetWork)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolGetEdition,
		Description: "Get the detail view of one edition: identity, titles and the current personal state (consumption, intention, publication, progress by unit, rating, tags, references, notes), in the shared edition DTO.",
		InputSchema: getEditionSchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleGetEdition)
}

// filterFromInput type-maps the tool input onto the shared library
// filter. It only decodes: a present but invalid value is rejected by the
// shared service, exactly like the HTTP layer.
func filterFromInput(in searchLibraryInput) app.LibraryFilter {
	f := app.LibraryFilter{}
	if in.Query != "" {
		f.Query = &in.Query
	}
	for _, k := range in.Kinds {
		f.Kinds = append(f.Kinds, domain.EditionKind(k))
	}
	for _, p := range in.Publications {
		f.Publications = append(f.Publications, domain.Publication(p))
	}
	for _, c := range in.Consumptions {
		f.Consumptions = append(f.Consumptions, domain.Consumption(c))
	}
	f.UnknownConsumption = in.UnknownConsumption
	for _, i := range in.Intentions {
		f.Intentions = append(f.Intentions, domain.Intention(i))
	}
	f.Tags = append(f.Tags, in.Tags...)
	f.MinRating = in.MinRating
	if in.Sort != "" {
		f.Sort = app.LibrarySort(in.Sort)
	}
	f.Limit = in.Limit
	if in.Cursor != "" {
		f.Cursor = &in.Cursor
	}
	return f
}

// handleSearchLibrary is the search_library tool. A success is the shared
// list payload as structured output; a failure is the typed wire error
// object as a tool error.
func (s *Server) handleSearchLibrary(ctx context.Context, _ *mcpsdk.CallToolRequest, in searchLibraryInput) (*mcpsdk.CallToolResult, app.ListPayload, error) {
	payload, err := s.Service().SearchLibrary(ctx, filterFromInput(in))
	if err != nil {
		return Failure(err), app.ListPayload{}, nil
	}
	return nil, payload, nil
}

// handleGetWork is the get_work tool.
func (s *Server) handleGetWork(ctx context.Context, _ *mcpsdk.CallToolRequest, in getWorkInput) (*mcpsdk.CallToolResult, app.WorkView, error) {
	view, err := s.Service().GetWork(ctx, in.WorkID)
	if err != nil {
		return Failure(err), app.WorkView{}, nil
	}
	return nil, view, nil
}

// handleGetEdition is the get_edition tool.
func (s *Server) handleGetEdition(ctx context.Context, _ *mcpsdk.CallToolRequest, in getEditionInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	view, err := s.Service().GetEdition(ctx, in.EditionID)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	return nil, view, nil
}
