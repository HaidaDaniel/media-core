package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
)

// Relation and origin lookup failures. Application services map these to
// the stable wire error codes; the store stays transport-agnostic.
var (
	ErrRelationNotFound = errors.New("store: relation not found")
	ErrOriginNotFound   = errors.New("store: origin claim not found")
	ErrInvalidRelation  = errors.New("store: invalid relation")
	ErrInvalidOrigin    = errors.New("store: invalid origin claim")
	ErrRelationCycle    = errors.New("store: confirmed relation would create a cycle")
	ErrRelationConflict = errors.New("store: relation conflict")
	ErrOriginConflict   = errors.New("store: origin claim conflict")
)

// RelationActor is the acting identity of one relation or origin
// operation.
type RelationActor struct {
	Principal string `json:"principal"`
	Origin    string `json:"origin"`
	Identity  string `json:"identity,omitempty"`
}

// RelationInput is one typed, scoped, provenance-bearing relation
// assertion. Both endpoints share the scope; symmetric types are stored
// in canonical order; the state is explicit (proposed or confirmed at
// creation, never inferred from the actor origin) and the confidence is an
// optional hint, never a threshold.
type RelationInput struct {
	Scope      domain.RelationScope
	Type       domain.RelationType
	FromID     string
	ToID       string
	State      domain.ClaimState
	Provenance string
	Confidence *float64
	ReplacesID *string
}

// RelationInfo is one stored relation row. A revoked relation keeps its
// row, state, replacement link and instants, never an erase.
type RelationInfo struct {
	ID          string
	Scope       domain.RelationScope
	Type        domain.RelationType
	FromID      string
	ToID        string
	State       domain.ClaimState
	Provenance  string
	Confidence  *float64
	ReplacesID  *string
	ConfirmedBy *string
	ConfirmedAt *time.Time
	CreatedBy   string
	CreatedAt   time.Time
	RevokedBy   *string
	RevokedAt   *time.Time
}

// RelationResult is the store outcome of one relation mutation: the edge
// plus its canonical from entity and the entity revision after the
// change, so the application can settle the journal and re-list the view.
type RelationResult struct {
	Relation RelationInfo
	Scope    domain.RelationScope
	EntityID string
	Revision int64
}

// OriginInput is one explicit origin assertion on an edition: the kind is
// asserted, never inferred from a media kind or a provider, and the state
// is explicit (proposed or confirmed at creation).
type OriginInput struct {
	EditionID  string
	Kind       domain.OriginKind
	State      domain.ClaimState
	Provenance string
	ReplacesID *string
}

// OriginInfo is one stored origin claim row; a revoked claim keeps its row
// and instants, never an erase.
type OriginInfo struct {
	ID          string
	EditionID   string
	Kind        domain.OriginKind
	State       domain.ClaimState
	Provenance  string
	ReplacesID  *string
	ConfirmedBy *string
	ConfirmedAt *time.Time
	CreatedBy   string
	CreatedAt   time.Time
	RevokedBy   *string
	RevokedAt   *time.Time
}

// OriginResult is the store outcome of one origin mutation: the claim plus
// the edition revision after the change.
type OriginResult struct {
	Claim    OriginInfo
	Revision int64
}

// validateRelationInput checks the relation shape before any journal key
// is reserved: a defined scope and type that may bind the scope, distinct
// non-empty endpoints, an explicit creation state and a non-empty
// provenance claim.
func validateRelationInput(in RelationInput) error {
	if !in.Scope.Valid() {
		return fmt.Errorf("%w: unknown scope %q", ErrInvalidRelation, in.Scope)
	}
	if !in.Type.Valid() {
		return fmt.Errorf("%w: unknown relation type %q", ErrInvalidRelation, in.Type)
	}
	if !in.Type.AllowsScope(in.Scope) {
		return fmt.Errorf("%w: %s has no %s scope", ErrInvalidRelation, in.Type, in.Scope)
	}
	if strings.TrimSpace(in.FromID) == "" || strings.TrimSpace(in.ToID) == "" {
		return fmt.Errorf("%w: both endpoints are required", ErrInvalidRelation)
	}
	if in.FromID == in.ToID {
		return fmt.Errorf("%w: self links are not allowed", ErrInvalidRelation)
	}
	if in.State != domain.ClaimProposed && in.State != domain.ClaimConfirmed {
		return fmt.Errorf("%w: a relation is created proposed or confirmed, not %s", ErrInvalidRelation, in.State)
	}
	if strings.TrimSpace(in.Provenance) == "" {
		return fmt.Errorf("%w: provenance is required", ErrInvalidRelation)
	}
	return nil
}

// validateOriginInput checks the origin claim shape before any journal key
// is reserved: a defined kind, an explicit creation state and a non-empty
// provenance claim.
func validateOriginInput(in OriginInput) error {
	if strings.TrimSpace(in.EditionID) == "" {
		return fmt.Errorf("%w: edition id is required", ErrInvalidOrigin)
	}
	if !in.Kind.Valid() {
		return fmt.Errorf("%w: unknown origin kind %q", ErrInvalidOrigin, in.Kind)
	}
	if in.State != domain.ClaimProposed && in.State != domain.ClaimConfirmed {
		return fmt.Errorf("%w: a claim is created proposed or confirmed, not %s", ErrInvalidOrigin, in.State)
	}
	if strings.TrimSpace(in.Provenance) == "" {
		return fmt.Errorf("%w: provenance is required", ErrInvalidOrigin)
	}
	return nil
}

// relationColumns is the full relation row in a stable order.
const relationColumns = `id, scope, relation, from_id, to_id, state, provenance, confidence, replaces_id, confirmed_by, confirmed_at, created_by, created_at, revoked_by, revoked_at`

type rowScanner interface {
	Scan(dest ...any) error
}

// scanRelationRow fills one relation row from a scanner.
func scanRelationRow(s rowScanner) (RelationInfo, error) {
	var (
		r          RelationInfo
		scope      string
		rel        string
		state      string
		conf       sql.NullFloat64
		replaces   sql.NullString
		confirmedB sql.NullString
		confirmedA sql.NullString
		createdAt  string
		revokedB   sql.NullString
		revokedA   sql.NullString
	)
	if err := s.Scan(&r.ID, &scope, &rel, &r.FromID, &r.ToID, &state, &r.Provenance,
		&conf, &replaces, &confirmedB, &confirmedA, &r.CreatedBy, &createdAt, &revokedB, &revokedA); err != nil {
		return RelationInfo{}, err
	}
	r.Scope = domain.RelationScope(scope)
	r.Type = domain.RelationType(rel)
	r.State = domain.ClaimState(state)
	if conf.Valid {
		r.Confidence = &conf.Float64
	}
	if replaces.Valid {
		r.ReplacesID = &replaces.String
	}
	if confirmedB.Valid {
		r.ConfirmedBy = &confirmedB.String
	}
	if confirmedA.Valid {
		t, err := parseInstant(confirmedA.String)
		if err != nil {
			return RelationInfo{}, fmt.Errorf("store: parse relation confirmed_at: %w", err)
		}
		r.ConfirmedAt = &t
	}
	t, err := parseInstant(createdAt)
	if err != nil {
		return RelationInfo{}, fmt.Errorf("store: parse relation created_at: %w", err)
	}
	r.CreatedAt = t
	if revokedB.Valid {
		r.RevokedBy = &revokedB.String
	}
	if revokedA.Valid {
		t, err := parseInstant(revokedA.String)
		if err != nil {
			return RelationInfo{}, fmt.Errorf("store: parse relation revoked_at: %w", err)
		}
		r.RevokedAt = &t
	}
	return r, nil
}

// originColumns is the full origin claim row in a stable order.
const originColumns = `id, edition_id, kind, state, provenance, replaces_id, confirmed_by, confirmed_at, created_by, created_at, revoked_by, revoked_at`

// scanOriginRow fills one origin claim row from a scanner.
func scanOriginRow(s rowScanner) (OriginInfo, error) {
	var (
		c          OriginInfo
		kind       string
		state      string
		replaces   sql.NullString
		confirmedB sql.NullString
		confirmedA sql.NullString
		createdAt  string
		revokedB   sql.NullString
		revokedA   sql.NullString
	)
	if err := s.Scan(&c.ID, &c.EditionID, &kind, &state, &c.Provenance,
		&replaces, &confirmedB, &confirmedA, &c.CreatedBy, &createdAt, &revokedB, &revokedA); err != nil {
		return OriginInfo{}, err
	}
	c.Kind = domain.OriginKind(kind)
	c.State = domain.ClaimState(state)
	if replaces.Valid {
		c.ReplacesID = &replaces.String
	}
	if confirmedB.Valid {
		c.ConfirmedBy = &confirmedB.String
	}
	if confirmedA.Valid {
		t, err := parseInstant(confirmedA.String)
		if err != nil {
			return OriginInfo{}, fmt.Errorf("store: parse claim confirmed_at: %w", err)
		}
		c.ConfirmedAt = &t
	}
	t, err := parseInstant(createdAt)
	if err != nil {
		return OriginInfo{}, fmt.Errorf("store: parse claim created_at: %w", err)
	}
	c.CreatedAt = t
	if revokedB.Valid {
		c.RevokedBy = &revokedB.String
	}
	if revokedA.Valid {
		t, err := parseInstant(revokedA.String)
		if err != nil {
			return OriginInfo{}, fmt.Errorf("store: parse claim revoked_at: %w", err)
		}
		c.RevokedAt = &t
	}
	return c, nil
}

// requireEntityRevision is the scope-aware lost-update guard: works use
// the work revision, editions the edition revision; a missing entity is a
// distinguishable not-found.
func requireEntityRevision(tx *sql.Tx, ctx context.Context, scope domain.RelationScope, entityID string, expectedRevision int64) (int64, error) {
	if scope == domain.ScopeWork {
		return requireRevision(tx, ctx, entityID, expectedRevision)
	}
	return requireEditionRevision(tx, ctx, entityID, expectedRevision)
}

// bumpEntityRevision is the scope-aware revision bump for the entity a
// relation or origin claim belongs to.
func (d *Database) bumpEntityRevision(tx *sql.Tx, ctx context.Context, scope domain.RelationScope, entityID string, revision int64, now time.Time) error {
	if scope == domain.ScopeWork {
		return bumpWorkRevision(tx, ctx, entityID, revision, now)
	}
	return d.bumpEditionRevision(tx, ctx, entityID, revision, now)
}

// entityResultKey is the journal summary key of the scoped entity.
func entityResultKey(scope domain.RelationScope) string {
	if scope == domain.ScopeWork {
		return "work_id"
	}
	return "edition_id"
}

// relationEndpointError verifies that one endpoint exists in the link
// scope: an ID of the other scope is a mixed-scope link, an absent ID is a
// distinguishable not-found.
func relationEndpointError(ctx context.Context, q queryer, scope domain.RelationScope, id string) error {
	if scope == domain.ScopeEdition {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM editions WHERE id = ?`, id).Scan(&n); err != nil {
			return fmt.Errorf("store: check relation endpoint: %w", err)
		}
		if n > 0 {
			return nil
		}
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM works WHERE id = ?`, id).Scan(&n); err != nil {
			return fmt.Errorf("store: check relation endpoint: %w", err)
		}
		if n > 0 {
			return fmt.Errorf("%w: mixed-scope link: %s is a work, not an edition", ErrInvalidRelation, id)
		}
		return fmt.Errorf("%w: %s", ErrEditionNotFound, id)
	}
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM works WHERE id = ?`, id).Scan(&n); err != nil {
		return fmt.Errorf("store: check relation endpoint: %w", err)
	}
	if n > 0 {
		return nil
	}
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM editions WHERE id = ?`, id).Scan(&n); err != nil {
		return fmt.Errorf("store: check relation endpoint: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("%w: mixed-scope link: %s is an edition, not a work", ErrInvalidRelation, id)
	}
	return fmt.Errorf("%w: %s", ErrWorkNotFound, id)
}

// confirmedCycle reports whether confirming the directed edge
// fromID -> toID would close a cycle in the type's confirmed graph: the
// combined adaptation_of + translation_of source graph, or the sequel and
// side-story graphs. Undirected types never cycle-check. The new edge is
// not yet inserted, so the search runs over the confirmed edges present in
// the transaction.
func confirmedCycle(ctx context.Context, q queryer, scope domain.RelationScope, typ domain.RelationType, fromID, toID string) (bool, error) {
	graph := typ.CycleGraph()
	if graph == "" {
		return false, nil
	}
	relations := []string{}
	switch graph {
	case "source":
		relations = []string{string(domain.RelationAdaptationOf), string(domain.RelationTranslationOf)}
	case "sequel":
		relations = []string{string(domain.RelationSequelOf)}
	case "side_story":
		relations = []string{string(domain.RelationSideStoryOf)}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(relations)), ",")
	rows, err := q.QueryContext(ctx,
		`SELECT from_id, to_id FROM media_relations WHERE scope = ? AND state = 'confirmed' AND relation IN (`+placeholders+`)`,
		append([]any{string(scope)}, toAnySlice(relations)...)...)
	if err != nil {
		return false, fmt.Errorf("store: load confirmed %s edges: %w", graph, err)
	}
	defer rows.Close()
	// Edges point adaptation -> source; following them from the new
	// edge's source reaches the new edge's adaptation exactly when the
	// source graph would loop back.
	adjacent := map[string][]string{}
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			return false, fmt.Errorf("store: scan confirmed %s edge: %w", graph, err)
		}
		adjacent[from] = append(adjacent[from], to)
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("store: scan confirmed %s edges: %w", graph, err)
	}
	visited := map[string]bool{toID: true}
	queue := []string{toID}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		for _, next := range adjacent[node] {
			if next == fromID {
				return true, nil
			}
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false, nil
}

func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, v)
	}
	return out
}

// findRelationAny loads the relation row in any lifecycle state, or nil.
func findRelationAny(ctx context.Context, q queryer, relationID string) (*RelationInfo, error) {
	row := q.QueryRowContext(ctx, `SELECT `+relationColumns+` FROM media_relations WHERE id = ?`, relationID)
	r, err := scanRelationRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find relation: %w", err)
	}
	return &r, nil
}

// findActiveEdge loads the non-revoked edge of the same scope, type and
// canonical endpoints, or nil.
func findActiveEdge(ctx context.Context, q queryer, scope domain.RelationScope, typ domain.RelationType, fromID, toID string) (*RelationInfo, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+relationColumns+` FROM media_relations
		 WHERE scope = ? AND relation = ? AND from_id = ? AND to_id = ? AND state != 'revoked'
		 LIMIT 1`, string(scope), string(typ), fromID, toID)
	r, err := scanRelationRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find active edge: %w", err)
	}
	return &r, nil
}

// AddRelation asserts one typed, scoped, provenance-bearing relation. The
// state is explicit: an agent guess arrives proposed and stays proposed
// until an explicit confirmation. A duplicate active edge of the same
// scope, type and canonical endpoints returns the existing edge without a
// revision bump; a replacement revokes the named active edge and inserts a
// new one with the replacement link, never a silent rewrite. Confirming a
// directed edge checks the combined confirmed source graph, the sequel
// graph and the side-story graph for cycles in the same write transaction;
// undirected edges may cycle. The canonical from entity's revision guards
// lost updates and the optional journal row commits with the change.
func (d *Database) AddRelation(ctx context.Context, now time.Time, in RelationInput, expectedRevision int64, actor RelationActor, journal *JournalInput) (RelationResult, error) {
	if err := validateRelationInput(in); err != nil {
		return RelationResult{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return RelationResult{}, err
	}
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return RelationResult{}, err
	}
	defer tx.Rollback()
	// The raw endpoints are scope-checked before canonical order, so a
	// mixed-scope link is a mixed-scope error, whatever the ID order.
	if err := relationEndpointError(ctx, tx, in.Scope, in.FromID); err != nil {
		return RelationResult{}, err
	}
	if err := relationEndpointError(ctx, tx, in.Scope, in.ToID); err != nil {
		return RelationResult{}, err
	}
	fromID, toID := in.FromID, in.ToID
	if in.Type.Symmetric() {
		fromID, toID = domain.CanonicalEndpoints(fromID, toID)
	}
	revision, err := requireEntityRevision(tx, ctx, in.Scope, fromID, expectedRevision)
	if err != nil {
		return RelationResult{}, err
	}
	if in.State == domain.ClaimConfirmed {
		cycle, err := confirmedCycle(ctx, tx, in.Scope, in.Type, fromID, toID)
		if err != nil {
			return RelationResult{}, err
		}
		if cycle {
			return RelationResult{}, fmt.Errorf("%w: %s %s -> %s loops the confirmed %s graph", ErrRelationCycle, in.Type, fromID, toID, in.Type.CycleGraph())
		}
	}
	replaced := false
	if in.ReplacesID != nil {
		target, err := findRelationAny(ctx, tx, *in.ReplacesID)
		if err != nil {
			return RelationResult{}, err
		}
		if target == nil {
			return RelationResult{}, fmt.Errorf("%w: replaces target %s not found", ErrRelationConflict, *in.ReplacesID)
		}
		if target.State == domain.ClaimRevoked {
			return RelationResult{}, fmt.Errorf("%w: replaces target %s is already revoked", ErrRelationConflict, *in.ReplacesID)
		}
		if target.Scope != in.Scope {
			return RelationResult{}, fmt.Errorf("%w: replaces target %s has a different scope", ErrRelationConflict, *in.ReplacesID)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE media_relations SET state = 'revoked', revoked_by = ?, revoked_at = ? WHERE id = ?`,
			actor.Principal, nowS, target.ID); err != nil {
			return RelationResult{}, fmt.Errorf("store: revoke replaced relation: %w", err)
		}
		replaced = true
	}
	existing, err := findActiveEdge(ctx, tx, in.Scope, in.Type, fromID, toID)
	if err != nil {
		return RelationResult{}, err
	}
	if existing != nil && !replaced {
		// A duplicate active edge returns the existing edge; the journal
		// still settles and no revision is bumped.
		if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON(entityResultKey(in.Scope), fromID, revision)); err != nil {
			return RelationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return RelationResult{}, fmt.Errorf("store: commit relation no-op: %w", err)
		}
		return RelationResult{Relation: *existing, Scope: in.Scope, EntityID: fromID, Revision: revision}, nil
	}
	if existing != nil {
		return RelationResult{}, fmt.Errorf("%w: an active %s edge already links %s and %s", ErrRelationConflict, in.Type, fromID, toID)
	}
	confirmedBy, confirmedAt := (*string)(nil), (*string)(nil)
	if in.State == domain.ClaimConfirmed {
		confirmedBy = &actor.Principal
		confirmedAt = &nowS
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO media_relations (id, scope, relation, from_id, to_id, state, provenance, confidence, replaces_id, confirmed_by, confirmed_at, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, string(in.Scope), string(in.Type), fromID, toID, string(in.State), in.Provenance, in.Confidence, in.ReplacesID, confirmedBy, confirmedAt, actor.Principal, nowS); err != nil {
		return RelationResult{}, fmt.Errorf("store: insert relation: %w", err)
	}
	if err := d.bumpEntityRevision(tx, ctx, in.Scope, fromID, revision, now); err != nil {
		return RelationResult{}, err
	}
	created, err := findRelationAny(ctx, tx, id)
	if err != nil {
		return RelationResult{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON(entityResultKey(in.Scope), fromID, revision+1)); err != nil {
		return RelationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RelationResult{}, fmt.Errorf("store: commit relation: %w", err)
	}
	return RelationResult{Relation: *created, Scope: in.Scope, EntityID: fromID, Revision: revision + 1}, nil
}

// ConfirmRelation explicitly confirms a proposed relation. The cycle check
// runs at confirmation, in the same write transaction, because the
// confirmed graph grows only here. Confirming an already-confirmed
// relation is a no-op without a revision bump; confirming a revoked one is
// a distinguishable conflict. The canonical from entity's revision guards
// lost updates.
func (d *Database) ConfirmRelation(ctx context.Context, now time.Time, relationID string, expectedRevision int64, actor RelationActor, journal *JournalInput) (RelationResult, error) {
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return RelationResult{}, err
	}
	defer tx.Rollback()
	rel, err := findRelationAny(ctx, tx, relationID)
	if err != nil {
		return RelationResult{}, err
	}
	if rel == nil {
		return RelationResult{}, fmt.Errorf("%w: %s", ErrRelationNotFound, relationID)
	}
	revision, err := requireEntityRevision(tx, ctx, rel.Scope, rel.FromID, expectedRevision)
	if err != nil {
		return RelationResult{}, err
	}
	if rel.State == domain.ClaimConfirmed {
		if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON(entityResultKey(rel.Scope), rel.FromID, revision)); err != nil {
			return RelationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return RelationResult{}, fmt.Errorf("store: commit relation confirmation no-op: %w", err)
		}
		return RelationResult{Relation: *rel, Scope: rel.Scope, EntityID: rel.FromID, Revision: revision}, nil
	}
	if rel.State == domain.ClaimRevoked {
		return RelationResult{}, fmt.Errorf("%w: relation %s is revoked and cannot be confirmed", ErrRelationConflict, relationID)
	}
	cycle, err := confirmedCycle(ctx, tx, rel.Scope, rel.Type, rel.FromID, rel.ToID)
	if err != nil {
		return RelationResult{}, err
	}
	if cycle {
		return RelationResult{}, fmt.Errorf("%w: %s %s -> %s loops the confirmed %s graph", ErrRelationCycle, rel.Type, rel.FromID, rel.ToID, rel.Type.CycleGraph())
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE media_relations SET state = 'confirmed', confirmed_by = ?, confirmed_at = ? WHERE id = ?`,
		actor.Principal, nowS, relationID); err != nil {
		return RelationResult{}, fmt.Errorf("store: confirm relation: %w", err)
	}
	if err := d.bumpEntityRevision(tx, ctx, rel.Scope, rel.FromID, revision, now); err != nil {
		return RelationResult{}, err
	}
	confirmed, err := findRelationAny(ctx, tx, relationID)
	if err != nil {
		return RelationResult{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON(entityResultKey(rel.Scope), rel.FromID, revision+1)); err != nil {
		return RelationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RelationResult{}, fmt.Errorf("store: commit relation confirmation: %w", err)
	}
	return RelationResult{Relation: *confirmed, Scope: rel.Scope, EntityID: rel.FromID, Revision: revision + 1}, nil
}

// RevokeRelation revokes an active relation: the row keeps its history,
// state and instants, and a replacement is a new edge, never a silent
// rewrite. Revoking an already-revoked relation is a no-op without a
// revision bump. The canonical from entity's revision guards lost updates.
func (d *Database) RevokeRelation(ctx context.Context, now time.Time, relationID string, expectedRevision int64, actor RelationActor, journal *JournalInput) (RelationResult, error) {
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return RelationResult{}, err
	}
	defer tx.Rollback()
	rel, err := findRelationAny(ctx, tx, relationID)
	if err != nil {
		return RelationResult{}, err
	}
	if rel == nil {
		return RelationResult{}, fmt.Errorf("%w: %s", ErrRelationNotFound, relationID)
	}
	revision, err := requireEntityRevision(tx, ctx, rel.Scope, rel.FromID, expectedRevision)
	if err != nil {
		return RelationResult{}, err
	}
	if rel.State == domain.ClaimRevoked {
		if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON(entityResultKey(rel.Scope), rel.FromID, revision)); err != nil {
			return RelationResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return RelationResult{}, fmt.Errorf("store: commit relation revocation no-op: %w", err)
		}
		return RelationResult{Relation: *rel, Scope: rel.Scope, EntityID: rel.FromID, Revision: revision}, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE media_relations SET state = 'revoked', revoked_by = ?, revoked_at = ? WHERE id = ?`,
		actor.Principal, nowS, relationID); err != nil {
		return RelationResult{}, fmt.Errorf("store: revoke relation: %w", err)
	}
	if err := d.bumpEntityRevision(tx, ctx, rel.Scope, rel.FromID, revision, now); err != nil {
		return RelationResult{}, err
	}
	revoked, err := findRelationAny(ctx, tx, relationID)
	if err != nil {
		return RelationResult{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON(entityResultKey(rel.Scope), rel.FromID, revision+1)); err != nil {
		return RelationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RelationResult{}, fmt.Errorf("store: commit relation revocation: %w", err)
	}
	return RelationResult{Relation: *revoked, Scope: rel.Scope, EntityID: rel.FromID, Revision: revision + 1}, nil
}

// ListRelations lists every relation (proposed, confirmed and revoked)
// touching the scoped entity in creation order; the application view
// separates the lifecycle states and revoked edges stay visible.
func (d *Database) ListRelations(ctx context.Context, scope domain.RelationScope, entityID string) ([]RelationInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT `+relationColumns+` FROM media_relations
		 WHERE scope = ? AND (from_id = ? OR to_id = ?)
		 ORDER BY created_at, id`, string(scope), entityID, entityID)
	if err != nil {
		return nil, fmt.Errorf("store: list relations: %w", err)
	}
	defer rows.Close()
	out := []RelationInfo{}
	for rows.Next() {
		r, err := scanRelationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RelatedEdge is one traversed relation with its hop depth from the start
// entity.
type RelatedEdge struct {
	RelationID string
	Type       domain.RelationType
	FromID     string
	ToID       string
	State      domain.ClaimState
	Depth      int
}

// RelatedMedia traverses the non-revoked relations of one scope in both
// directions with a bounded depth and a visited set, so undirected cycles
// terminate and no edge is revisited.
func (d *Database) RelatedMedia(ctx context.Context, scope domain.RelationScope, entityID string, depth int) ([]RelatedEdge, error) {
	if !scope.Valid() {
		return nil, fmt.Errorf("%w: unknown scope %q", ErrInvalidRelation, scope)
	}
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, relation, from_id, to_id, state FROM media_relations
		 WHERE scope = ? AND state != 'revoked'`, string(scope))
	if err != nil {
		return nil, fmt.Errorf("store: load relations for traversal: %w", err)
	}
	defer rows.Close()
	type rawEdge struct {
		id    string
		typ   domain.RelationType
		from  string
		to    string
		state domain.ClaimState
	}
	edges := []rawEdge{}
	for rows.Next() {
		var e rawEdge
		if err := rows.Scan(&e.id, &e.typ, &e.from, &e.to, &e.state); err != nil {
			return nil, fmt.Errorf("store: scan relation for traversal: %w", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: scan relations for traversal: %w", err)
	}
	// Adjacency: each edge touches both endpoints; the traversal walks
	// from a node to the other endpoint of any edge that touches it.
	adjacent := map[string][]string{}
	touch := func(a, b string) {
		adjacent[a] = append(adjacent[a], b)
	}
	for _, e := range edges {
		touch(e.from, e.to)
	}
	// Edges are reported once, at the first hop they are reached; nodes
	// are enqueued once, so undirected cycles terminate. An edge between
	// two already-visited nodes is still reported when it is first
	// touched.
	out := []RelatedEdge{}
	recorded := map[string]bool{}
	seen := map[string]bool{}
	seen[entityID] = true
	queue := []struct {
		node  string
		depth int
	}{{entityID, 0}}
	for len(queue) > 0 {
		step := queue[0]
		queue = queue[1:]
		if step.depth >= depth {
			continue
		}
		for _, edge := range edges {
			var neighbor string
			switch {
			case edge.from == step.node:
				neighbor = edge.to
			case edge.to == step.node:
				neighbor = edge.from
			default:
				continue
			}
			if !recorded[edge.id] {
				recorded[edge.id] = true
				out = append(out, RelatedEdge{RelationID: edge.id, Type: edge.typ, FromID: edge.from, ToID: edge.to, State: edge.state, Depth: step.depth + 1})
			}
			if !seen[neighbor] {
				seen[neighbor] = true
				queue = append(queue, struct {
					node  string
					depth int
				}{neighbor, step.depth + 1})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].RelationID < out[j].RelationID
	})
	return out, nil
}

// RecordOrigin asserts one explicit origin claim on an edition. The kind
// is asserted, never inferred from a media kind or a provider: an edition
// with no confirmed claim has an unknown origin by absence. A duplicate
// active claim of the same kind returns the existing claim for an exact
// re-assertion and conflicts otherwise; a replacement revokes the named
// active claim and inserts a new one with the replacement link, so the
// corrected history stays visible. The edition revision guards lost
// updates and the optional journal row commits with the change.
func (d *Database) RecordOrigin(ctx context.Context, now time.Time, in OriginInput, expectedRevision int64, actor RelationActor, journal *JournalInput) (OriginResult, error) {
	if err := validateOriginInput(in); err != nil {
		return OriginResult{}, err
	}
	id, err := domain.NewUUID()
	if err != nil {
		return OriginResult{}, err
	}
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return OriginResult{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, in.EditionID, expectedRevision)
	if err != nil {
		return OriginResult{}, err
	}
	replaced := false
	if in.ReplacesID != nil {
		target, err := findActiveOrigin(tx, ctx, in.EditionID, *in.ReplacesID)
		if err != nil {
			return OriginResult{}, err
		}
		if target == nil {
			return OriginResult{}, fmt.Errorf("%w: replaces target %s not found on this edition", ErrOriginConflict, *in.ReplacesID)
		}
		if target.State == domain.ClaimRevoked {
			return OriginResult{}, fmt.Errorf("%w: replaces target %s is already revoked", ErrOriginConflict, *in.ReplacesID)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE origin_claims SET state = 'revoked', revoked_by = ?, revoked_at = ? WHERE id = ?`,
			actor.Principal, nowS, target.ID); err != nil {
			return OriginResult{}, fmt.Errorf("store: revoke replaced claim: %w", err)
		}
		replaced = true
	}
	existing, err := findActiveOriginOfKind(tx, ctx, in.EditionID, in.Kind)
	if err != nil {
		return OriginResult{}, err
	}
	if existing != nil && !replaced {
		if existing.State == in.State && existing.Provenance == in.Provenance {
			// An exact re-assertion: a no-op without a revision bump; the
			// journal still settles.
			if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", in.EditionID, revision)); err != nil {
				return OriginResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return OriginResult{}, fmt.Errorf("store: commit origin no-op: %w", err)
			}
			return OriginResult{Claim: *existing, Revision: revision}, nil
		}
		return OriginResult{}, fmt.Errorf("%w: an active %s claim exists; replace it to correct it", ErrOriginConflict, in.Kind)
	}
	if existing != nil {
		return OriginResult{}, fmt.Errorf("%w: an active %s claim exists on this edition", ErrOriginConflict, in.Kind)
	}
	confirmedBy, confirmedAt := (*string)(nil), (*string)(nil)
	if in.State == domain.ClaimConfirmed {
		confirmedBy = &actor.Principal
		confirmedAt = &nowS
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO origin_claims (id, edition_id, kind, state, provenance, replaces_id, confirmed_by, confirmed_at, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.EditionID, string(in.Kind), string(in.State), in.Provenance, in.ReplacesID, confirmedBy, confirmedAt, actor.Principal, nowS); err != nil {
		return OriginResult{}, fmt.Errorf("store: insert origin claim: %w", err)
	}
	if err := d.bumpEditionRevision(tx, ctx, in.EditionID, revision, now); err != nil {
		return OriginResult{}, err
	}
	created, err := findOriginAny(tx, ctx, id)
	if err != nil {
		return OriginResult{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", in.EditionID, revision+1)); err != nil {
		return OriginResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OriginResult{}, fmt.Errorf("store: commit origin claim: %w", err)
	}
	return OriginResult{Claim: *created, Revision: revision + 1}, nil
}

// ConfirmOrigin explicitly confirms a proposed origin claim. Confirming an
// already-confirmed claim is a no-op without a revision bump; confirming a
// revoked one is a distinguishable conflict. The edition revision guards
// lost updates.
func (d *Database) ConfirmOrigin(ctx context.Context, now time.Time, editionID, claimID string, expectedRevision int64, actor RelationActor, journal *JournalInput) (OriginResult, error) {
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return OriginResult{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return OriginResult{}, err
	}
	claim, err := findOriginAny(tx, ctx, claimID)
	if err != nil {
		return OriginResult{}, err
	}
	if claim == nil || claim.EditionID != editionID {
		return OriginResult{}, fmt.Errorf("%w: %s", ErrOriginNotFound, claimID)
	}
	if claim.State == domain.ClaimConfirmed {
		if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision)); err != nil {
			return OriginResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return OriginResult{}, fmt.Errorf("store: commit origin confirmation no-op: %w", err)
		}
		return OriginResult{Claim: *claim, Revision: revision}, nil
	}
	if claim.State == domain.ClaimRevoked {
		return OriginResult{}, fmt.Errorf("%w: claim %s is revoked and cannot be confirmed", ErrOriginConflict, claimID)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE origin_claims SET state = 'confirmed', confirmed_by = ?, confirmed_at = ? WHERE id = ?`,
		actor.Principal, nowS, claimID); err != nil {
		return OriginResult{}, fmt.Errorf("store: confirm origin claim: %w", err)
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return OriginResult{}, err
	}
	confirmed, err := findOriginAny(tx, ctx, claimID)
	if err != nil {
		return OriginResult{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision+1)); err != nil {
		return OriginResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OriginResult{}, fmt.Errorf("store: commit origin confirmation: %w", err)
	}
	return OriginResult{Claim: *confirmed, Revision: revision + 1}, nil
}

// RevokeOrigin revokes an origin claim: the row keeps its history and
// instants, so the corrected origin stays visible. Revoking an
// already-revoked claim is a no-op without a revision bump. The edition
// revision guards lost updates.
func (d *Database) RevokeOrigin(ctx context.Context, now time.Time, editionID, claimID string, expectedRevision int64, actor RelationActor, journal *JournalInput) (OriginResult, error) {
	nowS := instantUTC(now)
	tx, err := d.beginImmediate(ctx)
	if err != nil {
		return OriginResult{}, err
	}
	defer tx.Rollback()
	revision, err := requireEditionRevision(tx, ctx, editionID, expectedRevision)
	if err != nil {
		return OriginResult{}, err
	}
	claim, err := findOriginAny(tx, ctx, claimID)
	if err != nil {
		return OriginResult{}, err
	}
	if claim == nil || claim.EditionID != editionID {
		return OriginResult{}, fmt.Errorf("%w: %s", ErrOriginNotFound, claimID)
	}
	if claim.State == domain.ClaimRevoked {
		if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision)); err != nil {
			return OriginResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return OriginResult{}, fmt.Errorf("store: commit origin revocation no-op: %w", err)
		}
		return OriginResult{Claim: *claim, Revision: revision}, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE origin_claims SET state = 'revoked', revoked_by = ?, revoked_at = ? WHERE id = ?`,
		actor.Principal, nowS, claimID); err != nil {
		return OriginResult{}, fmt.Errorf("store: revoke origin claim: %w", err)
	}
	if err := d.bumpEditionRevision(tx, ctx, editionID, revision, now); err != nil {
		return OriginResult{}, err
	}
	revoked, err := findOriginAny(tx, ctx, claimID)
	if err != nil {
		return OriginResult{}, err
	}
	if err := recordJournalOr(tx, ctx, journal, now, entityResultJSON("edition_id", editionID, revision+1)); err != nil {
		return OriginResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return OriginResult{}, fmt.Errorf("store: commit origin revocation: %w", err)
	}
	return OriginResult{Claim: *revoked, Revision: revision + 1}, nil
}

// ListOrigins lists every origin claim (proposed, confirmed and revoked)
// of an edition in creation order; revoked claims stay visible with their
// replacement links.
func (d *Database) ListOrigins(ctx context.Context, editionID string) ([]OriginInfo, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT `+originColumns+` FROM origin_claims WHERE edition_id = ? ORDER BY created_at, id`, editionID)
	if err != nil {
		return nil, fmt.Errorf("store: list origin claims: %w", err)
	}
	defer rows.Close()
	out := []OriginInfo{}
	for rows.Next() {
		c, err := scanOriginRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// findActiveOrigin loads the non-revoked claim with the given ID on the
// edition, or nil when absent, on another edition or already revoked.
func findActiveOrigin(q queryer, ctx context.Context, editionID, claimID string) (*OriginInfo, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+originColumns+` FROM origin_claims WHERE id = ? AND edition_id = ? AND state != 'revoked'`, claimID, editionID)
	c, err := scanOriginRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find active origin claim: %w", err)
	}
	return &c, nil
}

// findOriginAny loads the claim row in any lifecycle state, or nil.
func findOriginAny(q queryer, ctx context.Context, claimID string) (*OriginInfo, error) {
	row := q.QueryRowContext(ctx, `SELECT `+originColumns+` FROM origin_claims WHERE id = ?`, claimID)
	c, err := scanOriginRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find origin claim: %w", err)
	}
	return &c, nil
}

// findActiveOriginOfKind loads the non-revoked claim of the same kind on
// the edition, or nil.
func findActiveOriginOfKind(q queryer, ctx context.Context, editionID string, kind domain.OriginKind) (*OriginInfo, error) {
	row := q.QueryRowContext(ctx,
		`SELECT `+originColumns+` FROM origin_claims
		 WHERE edition_id = ? AND kind = ? AND state != 'revoked'
		 LIMIT 1`, editionID, string(kind))
	c, err := scanOriginRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find active origin kind: %w", err)
	}
	return &c, nil
}
