package app

import (
	"context"
	"strings"
	"time"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// Relation is one typed, scoped, provenance-bearing edge in the relation
// graph with its full lifecycle: a revoked edge keeps its state,
// replacement link and instants, never an erase.
type Relation struct {
	ID          string               `json:"id"`
	Scope       domain.RelationScope `json:"scope"`
	Type        domain.RelationType  `json:"type"`
	FromID      string               `json:"from_id"`
	ToID        string               `json:"to_id"`
	State       domain.ClaimState    `json:"state"`
	Provenance  string               `json:"provenance"`
	Confidence  *float64             `json:"confidence"`
	ReplacesID  *string              `json:"replaces_id"`
	ConfirmedBy *string              `json:"confirmed_by"`
	ConfirmedAt *time.Time           `json:"confirmed_at"`
	CreatedAt   time.Time            `json:"created_at"`
	RevokedBy   *string              `json:"revoked_by"`
	RevokedAt   *time.Time           `json:"revoked_at"`
}

// RelationView separates the lifecycle states of the relations touching
// one scoped entity: proposals are shown separately and never as
// confirmed facts, and revoked edges stay visible in history.
type RelationView struct {
	Proposed  []Relation `json:"proposed"`
	Confirmed []Relation `json:"confirmed"`
	Revoked   []Relation `json:"revoked"`
}

// OriginClaim is one explicit origin assertion on an edition. Unknown is
// the default absence of a confirmed origin; no originality is ever
// inferred from a media kind or a provider.
type OriginClaim struct {
	ID          string            `json:"id"`
	Kind        domain.OriginKind `json:"kind"`
	State       domain.ClaimState `json:"state"`
	Provenance  string            `json:"provenance"`
	ReplacesID  *string           `json:"replaces_id"`
	ConfirmedBy *string           `json:"confirmed_by"`
	ConfirmedAt *time.Time        `json:"confirmed_at"`
	CreatedAt   time.Time         `json:"created_at"`
	RevokedBy   *string           `json:"revoked_by"`
	RevokedAt   *time.Time        `json:"revoked_at"`
}

// OriginView lists every origin claim of an edition in creation order;
// revoked claims stay visible with their replacement links.
type OriginView struct {
	EditionID string        `json:"edition_id"`
	Claims    []OriginClaim `json:"claims"`
}

// RelatedEdge is one traversed relation with its hop depth from the start
// entity.
type RelatedEdge struct {
	RelationID string              `json:"relation_id"`
	Type       domain.RelationType `json:"type"`
	FromID     string              `json:"from_id"`
	ToID       string              `json:"to_id"`
	State      domain.ClaimState   `json:"state"`
	Depth      int                 `json:"depth"`
}

// RelatedView is the bounded both-directions traversal of the non-revoked
// relations of one scope, with a bounded depth and a visited set.
type RelatedView struct {
	EntityID string               `json:"entity_id"`
	Scope    domain.RelationScope `json:"scope"`
	Edges    []RelatedEdge        `json:"edges"`
}

// AddRelationRequest asserts one typed, scoped, provenance-bearing
// relation; the state is explicit and an agent guess arrives proposed.
type AddRelationRequest struct {
	MutationBase
	Scope      domain.RelationScope
	Type       domain.RelationType
	FromID     string
	ToID       string
	State      domain.ClaimState
	Provenance string
	Confidence *float64
	ReplacesID *string
}

// ConfirmRelationRequest explicitly confirms one proposed relation.
type ConfirmRelationRequest struct {
	MutationBase
	RelationID string
}

// RevokeRelationRequest revokes one active relation; the row keeps its
// history and a replacement is a new edge.
type RevokeRelationRequest struct {
	MutationBase
	RelationID string
}

// RecordOriginRequest asserts one explicit origin claim on an edition.
type RecordOriginRequest struct {
	MutationBase
	EditionID  string
	Kind       domain.OriginKind
	State      domain.ClaimState
	Provenance string
	ReplacesID *string
}

// ConfirmOriginRequest explicitly confirms one proposed origin claim.
type ConfirmOriginRequest struct {
	MutationBase
	EditionID string
	ClaimID   string
}

// RevokeOriginRequest revokes one origin claim; the row keeps its history.
type RevokeOriginRequest struct {
	MutationBase
	EditionID string
	ClaimID   string
}

// relationFrom maps one stored relation row to the frozen relation shape.
func relationFrom(r store.RelationInfo) Relation {
	return Relation{
		ID:          r.ID,
		Scope:       r.Scope,
		Type:        r.Type,
		FromID:      r.FromID,
		ToID:        r.ToID,
		State:       r.State,
		Provenance:  r.Provenance,
		Confidence:  r.Confidence,
		ReplacesID:  r.ReplacesID,
		ConfirmedBy: r.ConfirmedBy,
		ConfirmedAt: r.ConfirmedAt,
		CreatedAt:   r.CreatedAt,
		RevokedBy:   r.RevokedBy,
		RevokedAt:   r.RevokedAt,
	}
}

// originClaimFrom maps one stored claim row to the frozen claim shape.
func originClaimFrom(c store.OriginInfo) OriginClaim {
	return OriginClaim{
		ID:          c.ID,
		Kind:        c.Kind,
		State:       c.State,
		Provenance:  c.Provenance,
		ReplacesID:  c.ReplacesID,
		ConfirmedBy: c.ConfirmedBy,
		ConfirmedAt: c.ConfirmedAt,
		CreatedAt:   c.CreatedAt,
		RevokedBy:   c.RevokedBy,
		RevokedAt:   c.RevokedAt,
	}
}

// ListRelations lists the relations touching one scoped entity, separated
// by lifecycle state; read-only, no idempotency key.
func (s *Service) ListRelations(ctx context.Context, scope domain.RelationScope, entityID string) (RelationView, error) {
	if !scope.Valid() {
		return RelationView{}, invalidArgument("unknown relation scope "+string(scope), MutationBase{})
	}
	if strings.TrimSpace(entityID) == "" {
		return RelationView{}, invalidArgument("entity id is required", MutationBase{})
	}
	infos, err := s.db.ListRelations(ctx, scope, entityID)
	if err != nil {
		return RelationView{}, mapStoreError(err, MutationBase{})
	}
	view := RelationView{Proposed: []Relation{}, Confirmed: []Relation{}, Revoked: []Relation{}}
	for _, r := range infos {
		switch r.State {
		case domain.ClaimProposed:
			view.Proposed = append(view.Proposed, relationFrom(r))
		case domain.ClaimConfirmed:
			view.Confirmed = append(view.Confirmed, relationFrom(r))
		case domain.ClaimRevoked:
			view.Revoked = append(view.Revoked, relationFrom(r))
		}
	}
	return view, nil
}

// ListOrigins lists every origin claim of an edition, including revoked
// claims; read-only, no idempotency key.
func (s *Service) ListOrigins(ctx context.Context, editionID string) (OriginView, error) {
	if strings.TrimSpace(editionID) == "" {
		return OriginView{}, invalidArgument("edition id is required", MutationBase{})
	}
	claims, err := s.db.ListOrigins(ctx, editionID)
	if err != nil {
		return OriginView{}, mapStoreError(err, MutationBase{})
	}
	view := OriginView{EditionID: editionID, Claims: []OriginClaim{}}
	for _, c := range claims {
		view.Claims = append(view.Claims, originClaimFrom(c))
	}
	return view, nil
}

// Related traverses the non-revoked relations of one scope in both
// directions with a bounded depth and a visited set; read-only, no
// idempotency key.
func (s *Service) Related(ctx context.Context, scope domain.RelationScope, entityID string, depth int) (RelatedView, error) {
	if !scope.Valid() {
		return RelatedView{}, invalidArgument("unknown relation scope "+string(scope), MutationBase{})
	}
	if strings.TrimSpace(entityID) == "" {
		return RelatedView{}, invalidArgument("entity id is required", MutationBase{})
	}
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	edges, err := s.db.RelatedMedia(ctx, scope, entityID, depth)
	if err != nil {
		return RelatedView{}, mapStoreError(err, MutationBase{})
	}
	view := RelatedView{EntityID: entityID, Scope: scope, Edges: []RelatedEdge{}}
	for _, e := range edges {
		view.Edges = append(view.Edges, RelatedEdge{RelationID: e.RelationID, Type: e.Type, FromID: e.FromID, ToID: e.ToID, State: e.State, Depth: e.Depth})
	}
	return view, nil
}

// AddRelation asserts one typed, scoped, provenance-bearing relation and
// returns the relation view of its canonical from entity. A duplicate
// active edge returns the existing edge without a revision bump; a
// replacement revokes the named edge and inserts a new one; confirming a
// directed edge that would loop the confirmed source, sequel or
// side-story graph is a cycle error with no partial mutation.
func (s *Service) AddRelation(ctx context.Context, req AddRelationRequest) (RelationView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return RelationView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return RelationView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.FromID) == "" || strings.TrimSpace(req.ToID) == "" {
		return RelationView{}, invalidArgument("both relation endpoints are required", req.MutationBase)
	}
	if req.FromID == req.ToID {
		return RelationView{}, invalidArgument("self links are not allowed", req.MutationBase)
	}
	if !req.Type.Valid() {
		return RelationView{}, invalidArgument("unknown relation type "+string(req.Type), req.MutationBase)
	}
	if !req.Scope.Valid() {
		return RelationView{}, invalidArgument("unknown relation scope "+string(req.Scope), req.MutationBase)
	}
	if !req.Type.AllowsScope(req.Scope) {
		return RelationView{}, invalidArgument(string(req.Type)+" has no "+string(req.Scope)+" scope", req.MutationBase)
	}
	if req.State != domain.ClaimProposed && req.State != domain.ClaimConfirmed {
		return RelationView{}, invalidArgument("a relation is created proposed or confirmed, not "+string(req.State), req.MutationBase)
	}
	if strings.TrimSpace(req.Provenance) == "" {
		return RelationView{}, invalidArgument("provenance is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return RelationView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayRelationView(ctx, entry, req.MutationBase, hash)
	}
	res, err := s.db.AddRelation(ctx, s.now(), store.RelationInput{
		Scope:      req.Scope,
		Type:       req.Type,
		FromID:     req.FromID,
		ToID:       req.ToID,
		State:      req.State,
		Provenance: req.Provenance,
		Confidence: req.Confidence,
		ReplacesID: req.ReplacesID,
	}, *req.ExpectedRevision, store.RelationActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "add_relation",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleRelationView(ctx, req.MutationBase, hash, err)
	}
	return s.relationViewFor(ctx, res.Scope, res.EntityID)
}

// ConfirmRelation explicitly confirms one proposed relation and returns
// the relation view of its canonical from entity. The cycle check runs at
// confirmation, in the same write transaction.
func (s *Service) ConfirmRelation(ctx context.Context, req ConfirmRelationRequest) (RelationView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return RelationView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return RelationView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.RelationID) == "" {
		return RelationView{}, invalidArgument("relation id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return RelationView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayRelationView(ctx, entry, req.MutationBase, hash)
	}
	res, err := s.db.ConfirmRelation(ctx, s.now(), req.RelationID, *req.ExpectedRevision, store.RelationActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "confirm_relation",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleRelationView(ctx, req.MutationBase, hash, err)
	}
	return s.relationViewFor(ctx, res.Scope, res.EntityID)
}

// RevokeRelation revokes one active relation and returns the relation
// view of its canonical from entity; the row keeps its history.
func (s *Service) RevokeRelation(ctx context.Context, req RevokeRelationRequest) (RelationView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return RelationView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return RelationView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.RelationID) == "" {
		return RelationView{}, invalidArgument("relation id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return RelationView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayRelationView(ctx, entry, req.MutationBase, hash)
	}
	res, err := s.db.RevokeRelation(ctx, s.now(), req.RelationID, *req.ExpectedRevision, store.RelationActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "revoke_relation",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleRelationView(ctx, req.MutationBase, hash, err)
	}
	return s.relationViewFor(ctx, res.Scope, res.EntityID)
}

// RecordOrigin asserts one explicit origin claim on an edition and
// returns the origin view. No originality is inferred: a claim is created
// only with an explicit kind, state and provenance.
func (s *Service) RecordOrigin(ctx context.Context, req RecordOriginRequest) (OriginView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return OriginView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return OriginView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return OriginView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if !req.Kind.Valid() {
		return OriginView{}, invalidArgument("unknown origin kind "+string(req.Kind), req.MutationBase)
	}
	if req.State != domain.ClaimProposed && req.State != domain.ClaimConfirmed {
		return OriginView{}, invalidArgument("a claim is created proposed or confirmed, not "+string(req.State), req.MutationBase)
	}
	if strings.TrimSpace(req.Provenance) == "" {
		return OriginView{}, invalidArgument("provenance is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return OriginView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayOriginView(ctx, entry, req.MutationBase, hash)
	}
	_, err := s.db.RecordOrigin(ctx, s.now(), store.OriginInput{
		EditionID:  req.EditionID,
		Kind:       req.Kind,
		State:      req.State,
		Provenance: req.Provenance,
		ReplacesID: req.ReplacesID,
	}, *req.ExpectedRevision, store.RelationActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "record_origin",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleOriginView(ctx, req.MutationBase, hash, err)
	}
	return s.originViewFor(ctx, req.EditionID)
}

// ConfirmOrigin explicitly confirms one proposed origin claim and returns
// the origin view.
func (s *Service) ConfirmOrigin(ctx context.Context, req ConfirmOriginRequest) (OriginView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return OriginView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return OriginView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return OriginView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if strings.TrimSpace(req.ClaimID) == "" {
		return OriginView{}, invalidArgument("claim id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return OriginView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayOriginView(ctx, entry, req.MutationBase, hash)
	}
	_, err := s.db.ConfirmOrigin(ctx, s.now(), req.EditionID, req.ClaimID, *req.ExpectedRevision, store.RelationActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "confirm_origin",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleOriginView(ctx, req.MutationBase, hash, err)
	}
	return s.originViewFor(ctx, req.EditionID)
}

// RevokeOrigin revokes one origin claim and returns the origin view; the
// row keeps its history.
func (s *Service) RevokeOrigin(ctx context.Context, req RevokeOriginRequest) (OriginView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return OriginView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return OriginView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return OriginView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if strings.TrimSpace(req.ClaimID) == "" {
		return OriginView{}, invalidArgument("claim id is required", req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return OriginView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayOriginView(ctx, entry, req.MutationBase, hash)
	}
	_, err := s.db.RevokeOrigin(ctx, s.now(), req.EditionID, req.ClaimID, *req.ExpectedRevision, store.RelationActor{
		Principal: req.Actor.Principal,
		Origin:    string(req.Actor.Origin),
		Identity:  req.Actor.Identity,
	}, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "revoke_origin",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleOriginView(ctx, req.MutationBase, hash, err)
	}
	return s.originViewFor(ctx, req.EditionID)
}

// relationViewFor re-lists the relation view of the scoped entity a
// relation mutation touched.
func (s *Service) relationViewFor(ctx context.Context, scope domain.RelationScope, entityID string) (RelationView, error) {
	return s.ListRelations(ctx, scope, entityID)
}

// originViewFor re-lists the origin view of an edition.
func (s *Service) originViewFor(ctx context.Context, editionID string) (OriginView, error) {
	return s.ListOrigins(ctx, editionID)
}

// replayRelationView settles a same-key relation mutation against the
// journal: a matching hash re-derives the stored relation view of the
// scoped entity, a mismatched hash is a key-collision conflict, and a
// corrupt row is a storage error.
func (s *Service) replayRelationView(ctx context.Context, entry *store.JournalEntry, base MutationBase, hash string) (RelationView, error) {
	if entry.RequestHash != hash {
		return RelationView{}, conflict("the idempotency key was already used with a different payload", base)
	}
	sum, aerr := decodeEntitySummary(entry, base)
	if aerr != nil {
		return RelationView{}, aerr
	}
	if sum.WorkID != "" {
		return s.ListRelations(ctx, domain.ScopeWork, sum.WorkID)
	}
	return s.ListRelations(ctx, domain.ScopeEdition, sum.EditionID)
}

func (s *Service) settleRelationView(ctx context.Context, base MutationBase, hash string, err error) (RelationView, error) {
	if entry, lerr := s.db.LookupJournal(ctx, base.RequestID); lerr != nil {
		return RelationView{}, mapStoreError(lerr, base)
	} else if entry != nil {
		return s.replayRelationView(ctx, entry, base, hash)
	}
	return RelationView{}, mapStoreError(err, base)
}

// replayOriginView settles a same-key origin mutation against the journal:
// a matching hash re-derives the stored origin view of the edition, a
// mismatched hash is a key-collision conflict, and a corrupt row is a
// storage error.
func (s *Service) replayOriginView(ctx context.Context, entry *store.JournalEntry, base MutationBase, hash string) (OriginView, error) {
	if entry.RequestHash != hash {
		return OriginView{}, conflict("the idempotency key was already used with a different payload", base)
	}
	sum, aerr := decodeEntitySummary(entry, base)
	if aerr != nil {
		return OriginView{}, aerr
	}
	return s.ListOrigins(ctx, sum.EditionID)
}

func (s *Service) settleOriginView(ctx context.Context, base MutationBase, hash string, err error) (OriginView, error) {
	if entry, lerr := s.db.LookupJournal(ctx, base.RequestID); lerr != nil {
		return OriginView{}, mapStoreError(lerr, base)
	} else if entry != nil {
		return s.replayOriginView(ctx, entry, base, hash)
	}
	return OriginView{}, mapStoreError(err, base)
}
