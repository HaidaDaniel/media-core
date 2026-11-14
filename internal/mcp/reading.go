package mcp

import (
	"context"
	"time"

	"github.com/HaidaDaniel/media-core/internal/app"
	"github.com/HaidaDaniel/media-core/internal/domain"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Reading tool names.
const (
	ToolRecordProgress  = "record_progress"
	ToolSetConsumption  = "set_consumption"
	ToolRetractProgress = "retract_progress"
	ToolGetHistory      = "get_history"
)

// progressUnitSchema bounds the unit to the five defined values.
func progressUnitSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{
			string(domain.UnitChapter), string(domain.UnitVolume), string(domain.UnitEpisode),
			string(domain.UnitPage), string(domain.UnitPercent),
		},
	}
}

// consumptionValueSchema bounds the value to the five consumption states
// plus the explicit retraction value unknown.
func consumptionValueSchema() map[string]any {
	return map[string]any{
		"type": "string",
		"enum": []any{
			string(domain.ConsumptionNotStarted), string(domain.ConsumptionInProgress),
			string(domain.ConsumptionPaused), string(domain.ConsumptionCompleted),
			string(domain.ConsumptionDropped), "unknown",
		},
	}
}

var recordProgressSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "edition_id", "unit", "position", "expected_revision"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"edition_id":        map[string]any{"type": "string", "minLength": 1},
		"unit":              progressUnitSchema(),
		"position":          map[string]any{"type": "string", "minLength": 1},
		"total":             map[string]any{"type": []any{"null", "string"}},
		"label":             map[string]any{"type": []any{"null", "string"}},
		"observed_at":       map[string]any{"type": []any{"null", "string"}, "format": "date-time"},
		"reason":            map[string]any{"type": []any{"null", "string"}},
		"supersedes_id":     map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var setConsumptionSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "edition_id", "value", "expected_revision"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"edition_id":        map[string]any{"type": "string", "minLength": 1},
		"value":             consumptionValueSchema(),
		"reason":            map[string]any{"type": []any{"null", "string"}},
		"observed_at":       map[string]any{"type": []any{"null", "string"}, "format": "date-time"},
		"supersedes_id":     map[string]any{"type": []any{"null", "string"}},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var retractProgressSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []any{"request_id", "actor", "edition_id", "record_id", "reason", "expected_revision"},
	"properties": map[string]any{
		"request_id":        map[string]any{"type": "string", "minLength": 1},
		"actor":             actorSchema(),
		"edition_id":        map[string]any{"type": "string", "minLength": 1},
		"record_id":         map[string]any{"type": "string", "minLength": 1},
		"reason":            map[string]any{"type": "string", "minLength": 1},
		"expected_revision": map[string]any{"type": "integer"},
	},
}

var getHistorySchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"work_id":    map[string]any{"type": []any{"null", "string"}},
		"edition_id": map[string]any{"type": []any{"null", "string"}},
		"limit":      map[string]any{"type": "integer", "minimum": 0, "maximum": app.MaxActivityLimit},
		"cursor":     map[string]any{"type": []any{"null", "string"}},
	},
}

type recordProgressInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	EditionID        string     `json:"edition_id"`
	Unit             string     `json:"unit"`
	Position         string     `json:"position"`
	Total            *string    `json:"total"`
	Label            *string    `json:"label"`
	ObservedAt       *string    `json:"observed_at"`
	Reason           *string    `json:"reason"`
	SupersedesID     *string    `json:"supersedes_id"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type setConsumptionInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	EditionID        string     `json:"edition_id"`
	Value            string     `json:"value"`
	Reason           *string    `json:"reason"`
	ObservedAt       *string    `json:"observed_at"`
	SupersedesID     *string    `json:"supersedes_id"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type retractProgressInput struct {
	RequestID        string     `json:"request_id"`
	Actor            actorInput `json:"actor"`
	EditionID        string     `json:"edition_id"`
	RecordID         string     `json:"record_id"`
	Reason           string     `json:"reason"`
	ExpectedRevision *int64     `json:"expected_revision"`
}

type getHistoryInput struct {
	WorkID    *string `json:"work_id"`
	EditionID *string `json:"edition_id"`
	Limit     int     `json:"limit"`
	Cursor    *string `json:"cursor"`
}

func init() {
	trackTools(ToolRecordProgress, ToolSetConsumption, ToolRetractProgress, ToolGetHistory)
}

// RegisterReadingTools installs the reading tools. Every observation and
// correction routes through the shared service; the transport validates
// only the schema shape and defers all semantic rules (the unit bounds,
// the exact decimals, the revision guard, the journal) to the service.
// Progress and consumption are independent: a reading statement never
// means completed.
func RegisterReadingTools(s *Server) {
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolRecordProgress,
		Description: "Record one progress observation for an edition and a unit (chapter, volume, episode, page or percent). Position is an exact decimal string (up to 3 fractional digits); total is optional and never invented, and must not be supplied for percent (implicit 100). Lower positions are allowed for rereads. A supersedes_id correction retracts the named row of the same unit, which stays in history. Progress never changes consumption, intention or publication — a reading statement never means completed. expected_revision comes from the current edition view; a stale revision is a conflict. The same request ID with the same payload replays the stored result.",
		InputSchema: recordProgressSchema,
		Annotations: writeAnnotations(),
	}, s.handleRecordProgress)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolSetConsumption,
		Description: "Set the personal consumption state of an edition: not_started, in_progress, paused, completed or dropped, or the explicit retraction value unknown (which clears the state). A supersedes_id correction retracts the named row, which stays in history. Consumption never changes progress, intention or publication. expected_revision comes from the current edition view; a stale revision is a conflict. The same request ID with the same payload replays the stored result.",
		InputSchema: setConsumptionSchema,
		Annotations: writeAnnotations(),
	}, s.handleSetConsumption)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolRetractProgress,
		Description: "Retract one progress observation by its record id, with a required reason. The retracted row stays in history superseded, the unit's current position falls back to the next live observation or to absence, and the edition revision guards against lost updates. expected_revision comes from the current edition view; a stale revision is a conflict.",
		InputSchema: retractProgressSchema,
		Annotations: writeAnnotations(),
	}, s.handleRetractProgress)
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolGetHistory,
		Description: "Read the bounded complete observation history of exactly one work or one edition, with cursor pagination. Superseded and retracted rows stay visible with their correction label, so a correction is shown, never hidden or deleted. limit 0 uses the default page; the cursor is opaque and bound to this request.",
		InputSchema: getHistorySchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleGetHistory)
}

// parseInstant turns the transport's RFC 3339 string into the service's
// instant; an absent value stays zero.
func parseInstant(value *string) (time.Time, error) {
	if value == nil || *value == "" {
		return time.Time{}, nil
	}
	ts, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return time.Time{}, app.NewAppError(app.ErrInvalidArgument, "observed_at must be an RFC 3339 instant")
	}
	return ts, nil
}

// handleRecordProgress is the record_progress tool.
func (s *Server) handleRecordProgress(ctx context.Context, _ *mcpsdk.CallToolRequest, in recordProgressInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	observedAt, err := parseInstant(in.ObservedAt)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	req := app.RecordProgressRequest{
		MutationBase: app.MutationBase{
			RequestID:        in.RequestID,
			Actor:            in.Actor.toApp(),
			ExpectedRevision: in.ExpectedRevision,
		},
		EditionID:    in.EditionID,
		Unit:         domain.ProgressUnit(in.Unit),
		Position:     in.Position,
		Total:        in.Total,
		Label:        in.Label,
		ObservedAt:   observedAt,
		Reason:       in.Reason,
		SupersedesID: in.SupersedesID,
	}
	res, err := s.Service().RecordProgress(ctx, req)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	return nil, res, nil
}

// handleSetConsumption is the set_consumption tool.
func (s *Server) handleSetConsumption(ctx context.Context, _ *mcpsdk.CallToolRequest, in setConsumptionInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	var observedAt *time.Time
	if in.ObservedAt != nil && *in.ObservedAt != "" {
		ts, err := parseInstant(in.ObservedAt)
		if err != nil {
			return Failure(err), app.EditionView{}, nil
		}
		observedAt = &ts
	}
	req := app.SetConsumptionRequest{
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
	res, err := s.Service().SetConsumption(ctx, req)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	return nil, res, nil
}

// handleRetractProgress is the retract_progress tool: the typed retraction
// of one named progress record.
func (s *Server) handleRetractProgress(ctx context.Context, _ *mcpsdk.CallToolRequest, in retractProgressInput) (*mcpsdk.CallToolResult, app.EditionView, error) {
	req := app.RetractProgressRequest{
		MutationBase: app.MutationBase{
			RequestID:        in.RequestID,
			Actor:            in.Actor.toApp(),
			ExpectedRevision: in.ExpectedRevision,
		},
		EditionID: in.EditionID,
		RecordID:  in.RecordID,
		Reason:    in.Reason,
	}
	res, err := s.Service().RetractProgress(ctx, req)
	if err != nil {
		return Failure(err), app.EditionView{}, nil
	}
	return nil, res, nil
}

// handleGetHistory is the get_history tool. The exactly-one-target rule
// and the cursor binding are the service's; the transport passes them
// through.
func (s *Server) handleGetHistory(_ context.Context, _ *mcpsdk.CallToolRequest, in getHistoryInput) (*mcpsdk.CallToolResult, app.ListPayload, error) {
	res, err := s.Service().GetHistory(context.Background(), app.HistoryQuery{
		WorkID:    in.WorkID,
		EditionID: in.EditionID,
		Limit:     in.Limit,
		Cursor:    in.Cursor,
	})
	if err != nil {
		return Failure(err), app.ListPayload{}, nil
	}
	return nil, res, nil
}
