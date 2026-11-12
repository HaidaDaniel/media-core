package cli

import (
	"encoding/json"
	"strconv"
	"testing"
)

type relationData struct {
	Proposed  []map[string]any `json:"proposed"`
	Confirmed []map[string]any `json:"confirmed"`
	Revoked   []map[string]any `json:"revoked"`
}

type originData struct {
	EditionID string           `json:"edition_id"`
	Claims    []map[string]any `json:"claims"`
}

type relatedData struct {
	EntityID string           `json:"entity_id"`
	Scope    string           `json:"scope"`
	Edges    []map[string]any `json:"edges"`
}

func relationEnv(t *testing.T, out string) relationData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("relation envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data relationData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a relation view: %v\n%s", err, out)
	}
	return data
}

func originEnv(t *testing.T, out string) originData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("origin envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data originData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not an origin view: %v\n%s", err, out)
	}
	return data
}

func relatedEnv(t *testing.T, out string) relatedData {
	t.Helper()
	env := libEnv(t, out)
	if !env.OK || env.Error != nil {
		t.Fatalf("related envelope = %+v, want ok", env)
	}
	b, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var data relatedData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("data is not a related view: %v\n%s", err, out)
	}
	return data
}

func failCode(t *testing.T, out string) string {
	t.Helper()
	env := libEnv(t, out)
	if env.OK || env.Error == nil {
		t.Fatalf("envelope = %+v, want a failure", env)
	}
	return env.Error.Code
}

// TestRelationCLI proves the link/origin/related commands: a confirmed
// adaptation edge renders in the view, a duplicate active edge returns the
// existing edge, the proposed edge confirms and revokes, the related read
// traverses both directions, and the origin claim lifecycle works through
// the CLI.
func TestRelationCLI(t *testing.T) {
	dir := initLibState(t)
	_, edA := addWorkWithEdition(t, dir, "Anime Line")
	_, edB := addWorkWithEdition(t, dir, "Manga Source")

	// A confirmed adaptation edge: the view carries it.
	code, out, errw := runCLI(t, "link", "add", "--state-dir", dir, edA, edB,
		"--scope", "edition", "--type", "adaptation_of", "--state", "confirmed",
		"--provenance", "manual", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("link add: exit = %d, stderr = %q", code, errw)
	}
	data := relationEnv(t, out)
	if len(data.Confirmed) != 1 {
		t.Fatalf("view = %+v, want the confirmed edge", data)
	}
	edgeID, _ := data.Confirmed[0]["id"].(string)
	if data.Confirmed[0]["from_id"] != edA || data.Confirmed[0]["to_id"] != edB {
		t.Fatalf("edge = %+v, want adaptation -> source direction", data.Confirmed[0])
	}
	// A duplicate active edge returns the existing edge.
	code, out, errw = runCLI(t, "link", "add", "--state-dir", dir, edA, edB,
		"--scope", "edition", "--type", "adaptation_of", "--state", "confirmed",
		"--provenance", "manual", "--revision", "1")
	if code != ExitOK {
		t.Fatalf("duplicate link add: exit = %d, stderr = %q", code, errw)
	}
	data = relationEnv(t, out)
	if len(data.Confirmed) != 1 || data.Confirmed[0]["id"] != edgeID {
		t.Fatalf("view = %+v, want the existing edge", data)
	}
	// A proposed symmetric edge: the guard is the canonical from entity,
	// whose revision is known from the earlier directed edge.
	canonRev := int64(1)
	if edB < edA {
		canonRev = 0
	}
	code, out, errw = runCLI(t, "link", "add", "--state-dir", dir, edB, edA,
		"--scope", "edition", "--type", "related", "--provenance", "manual",
		"--revision", strconv.FormatInt(canonRev, 10))
	if code != ExitOK {
		t.Fatalf("proposed link add: exit = %d, stderr = %q", code, errw)
	}
	data = relationEnv(t, out)
	if len(data.Proposed) != 1 {
		t.Fatalf("view = %+v, want the proposed edge", data)
	}
	propID, _ := data.Proposed[0]["id"].(string)
	code, out, errw = runCLI(t, "link", "confirm", "--state-dir", dir, propID,
		"--revision", strconv.FormatInt(canonRev+1, 10))
	if code != ExitOK {
		t.Fatalf("link confirm: exit = %d, stderr = %q", code, errw)
	}
	data = relationEnv(t, out)
	if len(data.Confirmed) != 2 {
		t.Fatalf("view = %+v, want both edges confirmed", data)
	}
	code, out, errw = runCLI(t, "link", "revoke", "--state-dir", dir, propID,
		"--revision", strconv.FormatInt(canonRev+2, 10))
	if code != ExitOK {
		t.Fatalf("link revoke: exit = %d, stderr = %q", code, errw)
	}
	data = relationEnv(t, out)
	if len(data.Revoked) != 1 || len(data.Confirmed) != 1 {
		t.Fatalf("view = %+v, want the revoked edge kept in history", data)
	}
	// The related read traverses both directions.
	code, out, errw = runCLI(t, "related", "--state-dir", dir, edA, "--scope", "edition", "--depth", "2")
	if code != ExitOK {
		t.Fatalf("related: exit = %d, stderr = %q", code, errw)
	}
	rel := relatedEnv(t, out)
	if len(rel.Edges) != 1 || rel.Edges[0]["type"] != "adaptation_of" {
		t.Fatalf("related = %+v, want the single non-revoked edge", rel)
	}
	// The origin claim lifecycle through the CLI on a fresh edition.
	_, edC := addWorkWithEdition(t, dir, "Origin Edition")
	code, out, errw = runCLI(t, "origin", "set", "--state-dir", dir, edC,
		"--kind", "anime_original", "--state", "proposed", "--provenance", "manual", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("origin set: exit = %d, stderr = %q", code, errw)
	}
	odata := originEnv(t, out)
	if len(odata.Claims) != 1 || odata.Claims[0]["state"] != "proposed" {
		t.Fatalf("origin = %+v, want the proposed claim", odata)
	}
	claimID, _ := odata.Claims[0]["id"].(string)
	code, out, errw = runCLI(t, "origin", "confirm", "--state-dir", dir, edC,
		"--claim-id", claimID, "--revision", "1")
	if code != ExitOK {
		t.Fatalf("origin confirm: exit = %d, stderr = %q", code, errw)
	}
	odata = originEnv(t, out)
	if len(odata.Claims) != 1 || odata.Claims[0]["state"] != "confirmed" {
		t.Fatalf("origin = %+v, want the confirmed claim", odata)
	}
	code, out, errw = runCLI(t, "origin", "revoke", "--state-dir", dir, edC,
		"--claim-id", claimID, "--revision", "2")
	if code != ExitOK {
		t.Fatalf("origin revoke: exit = %d, stderr = %q", code, errw)
	}
	odata = originEnv(t, out)
	if len(odata.Claims) != 1 || odata.Claims[0]["state"] != "revoked" {
		t.Fatalf("origin = %+v, want the revoked claim kept in history", odata)
	}
}

// TestRelationCLIInvalid proves the CLI rejects malformed invocations as
// usage errors, self links and mixed-scope links as invalid arguments, a
// cyclic confirmed source link as a cycle failure with the prior state
// intact, and a stale revision as a conflict.
func TestRelationCLIInvalid(t *testing.T) {
	dir := initLibState(t)
	_, edA := addWorkWithEdition(t, dir, "Invalid A")
	_, edB := addWorkWithEdition(t, dir, "Invalid B")

	if code, _, _ := runCLI(t, "link", "rename", "--state-dir", dir, edA, edB); code != ExitUsage {
		t.Fatalf("unknown verb: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "link", "add", "--state-dir", dir, edA, edB, "--type", "related", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("missing scope: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "link", "add", "--state-dir", dir, edA, edB, "--scope", "edition", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("missing type: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "link", "add", "--state-dir", dir, edA, edB, "--scope", "edition", "--type", "related"); code != ExitUsage {
		t.Fatalf("missing provenance: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "link", "add", "--state-dir", dir, edA, edB, "--scope", "edition", "--type", "hologram", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("unknown type: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "link", "add", "--state-dir", dir, edA, edB, "--scope", "galaxy", "--type", "related", "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("unknown scope: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "related", "--state-dir", dir, edA); code != ExitUsage {
		t.Fatalf("missing related scope: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "origin", "set", "--state-dir", dir, edA, "--provenance", "manual"); code != ExitUsage {
		t.Fatalf("missing origin kind: exit = %d, want %d", code, ExitUsage)
	}
	if code, _, _ := runCLI(t, "origin", "confirm", "--state-dir", dir, edA); code != ExitUsage {
		t.Fatalf("missing claim id: exit = %d, want %d", code, ExitUsage)
	}

	// A self link is a stable invalid-argument failure.
	code, out, errw := runCLI(t, "link", "add", "--state-dir", dir, edA, edA,
		"--scope", "edition", "--type", "related", "--provenance", "manual", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("self link: exit = %d, want %d (stderr %q)", code, ExitUsage, errw)
	}
	if got := failCode(t, out); got != "invalid_argument" {
		t.Fatalf("self link code = %q, want invalid_argument", got)
	}
	// A mixed-scope link is a stable invalid-argument failure.
	edCWork := addWorkID(t, dir, "Mixed Work")
	code, out, errw = runCLI(t, "link", "add", "--state-dir", dir, edA, edCWork,
		"--scope", "edition", "--type", "related", "--provenance", "manual", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("mixed scope: exit = %d, want %d (stderr %q)", code, ExitUsage, errw)
	}
	if got := failCode(t, out); got != "invalid_argument" {
		t.Fatalf("mixed scope code = %q, want invalid_argument", got)
	}
	// A stale revision conflicts.
	code, out, errw = runCLI(t, "link", "add", "--state-dir", dir, edA, edB,
		"--scope", "edition", "--type", "related", "--provenance", "manual", "--revision", "99")
	if code != ExitUsage {
		t.Fatalf("stale revision: exit = %d, want %d (stderr %q)", code, ExitUsage, errw)
	}
	if got := failCode(t, out); got != "conflict" {
		t.Fatalf("stale revision code = %q, want conflict", got)
	}
	// A cyclic confirmed source link is a cycle failure with the prior
	// state intact.
	code, out, errw = runCLI(t, "link", "add", "--state-dir", dir, edA, edB,
		"--scope", "edition", "--type", "adaptation_of", "--state", "confirmed", "--provenance", "manual", "--revision", "0")
	if code != ExitOK {
		t.Fatalf("first edge: exit = %d, stderr = %q", code, errw)
	}
	code, out, errw = runCLI(t, "link", "add", "--state-dir", dir, edB, edA,
		"--scope", "edition", "--type", "adaptation_of", "--state", "confirmed", "--provenance", "manual", "--revision", "0")
	if code != ExitUsage {
		t.Fatalf("cyclic edge: exit = %d, want %d (stderr %q)", code, ExitUsage, errw)
	}
	if got := failCode(t, out); got != "cycle_detected" {
		t.Fatalf("cyclic edge code = %q, want cycle_detected", got)
	}
	// The first edge is intact.
	code, out, errw = runCLI(t, "related", "--state-dir", dir, edA, "--scope", "edition", "--depth", "1")
	if code != ExitOK {
		t.Fatalf("related after cycle: exit = %d, stderr = %q", code, errw)
	}
	rel := relatedEnv(t, out)
	if len(rel.Edges) != 1 || rel.Edges[0]["state"] != "confirmed" {
		t.Fatalf("related = %+v, want the intact first edge", rel)
	}
}
