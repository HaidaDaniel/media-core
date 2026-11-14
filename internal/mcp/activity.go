package mcp

import (
	"context"

	"github.com/HaidaDaniel/media-core/internal/app"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// ToolListActivity is the bounded recent-activity feed tool.
const ToolListActivity = "list_activity"

// activityTypesSchema bounds the filter to the defined v1 activity types.
func activityTypesSchema() map[string]any {
	return map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
			"enum": []any{
				"consumption", "progress", "intention", "rating", "publication",
				"note", "reference", "relation", "origin", "tag", "imported_release",
			},
		},
	}
}

var listActivitySchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"properties": map[string]any{
		"types":  activityTypesSchema(),
		"limit":  map[string]any{"type": "integer", "minimum": 0, "maximum": app.MaxActivityLimit},
		"cursor": map[string]any{"type": []any{"null", "string"}},
	},
}

type listActivityInput struct {
	Types  []string `json:"types"`
	Limit  int      `json:"limit"`
	Cursor *string  `json:"cursor"`
}

func init() {
	trackTools(ToolListActivity)
}

// RegisterActivityTools installs the bounded recent-activity feed over
// every personal history source. Reads never mutate; the type filter,
// the bound and the cursor binding are the shared service's.
func RegisterActivityTools(s *Server) {
	AddTool(s, &mcpsdk.Tool{
		Name:        ToolListActivity,
		Description: "Read the bounded recent personal activity over every history source, newest first, optionally filtered by activity type. limit 0 uses the default page; the cursor is opaque and bound to this request. Superseded and retracted rows stay visible with their correction label. Reads never mutate.",
		InputSchema: listActivitySchema,
		Annotations: readOnlyAnnotations(),
	}, s.handleListActivity)
}

// handleListActivity is the list_activity tool.
func (s *Server) handleListActivity(_ context.Context, _ *mcpsdk.CallToolRequest, in listActivityInput) (*mcpsdk.CallToolResult, app.ListPayload, error) {
	types := make([]app.ActivityType, 0, len(in.Types))
	for _, t := range in.Types {
		types = append(types, app.ActivityType(t))
	}
	res, err := s.Service().ListActivity(context.Background(), app.ActivityQuery{
		Types:  types,
		Limit:  in.Limit,
		Cursor: in.Cursor,
	})
	if err != nil {
		return Failure(err), app.ListPayload{}, nil
	}
	return nil, res, nil
}
