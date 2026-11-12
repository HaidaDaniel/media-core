package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HaidaDaniel/media-core/internal/domain"
	"github.com/HaidaDaniel/media-core/internal/store"
)

// LibrarySort is the requested ordering of a library listing.
type LibrarySort string

// Sort values.
const (
	SortTitle     LibrarySort = "title"
	SortCreatedAt LibrarySort = "created_at"
	SortUpdatedAt LibrarySort = "updated_at"
)

// Valid reports whether the sort is a defined v1 value.
func (s LibrarySort) Valid() bool {
	switch s {
	case SortTitle, SortCreatedAt, SortUpdatedAt:
		return true
	}
	return false
}

const (
	DefaultLibraryLimit  = 30
	MaxLibraryLimit      = 100
	MaxLibraryQueryRunes = 200
)

// LibraryFilter is one bounded library search request. All dimensions are
// optional; an empty filter lists the whole library. The cursor is opaque
// and bound to the sort, limit and filter fingerprint.
type LibraryFilter struct {
	Query *string
	Kinds []domain.EditionKind
	// Publications is the publication dimension; Consumptions and
	// UnknownConsumption are the consumption dimension, where unknown
	// selects editions whose current effective consumption is unknown;
	// Intentions is the intention dimension, where none selects editions
	// without a recorded intention; Tags holds tag names that are
	// normalized before matching, satisfied by a work tag or the matching
	// edition's own tag.
	Publications       []domain.Publication
	Consumptions       []domain.Consumption
	UnknownConsumption bool
	Intentions         []domain.Intention
	Tags               []string
	Sort               LibrarySort
	Limit              int
	Cursor             *string
}

// WorkCard is one search result: the work with its active titles and
// edition summaries, plus the editions that matched the query.
type WorkCard struct {
	WorkID            string           `json:"work_id"`
	Revision          int64            `json:"revision"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	ArchivedAt        *time.Time       `json:"archived_at"`
	Titles            []Title          `json:"titles"`
	Editions          []EditionSummary `json:"editions"`
	MatchedEditionIDs []string         `json:"matched_edition_ids"`
}

// libraryCursorPayload binds a seek point to the listing it belongs to.
type libraryCursorPayload struct {
	V     int    `json:"v"`
	Sort  string `json:"sort"`
	Limit int    `json:"limit"`
	QHash string `json:"q"`
	Key   string `json:"key"`
	ID    string `json:"id"`
}

// filterFingerprint is the canonical identity of the non-cursor part of a
// listing request; a cursor from a different request is rejected.
func filterFingerprint(query string, kinds []domain.EditionKind, publications []domain.Publication, consumptions []domain.Consumption, unknownConsumption bool, intentions []domain.Intention, tags []string) string {
	ks := make([]string, 0, len(kinds))
	for _, k := range kinds {
		ks = append(ks, string(k))
	}
	sort.Strings(ks)
	ps := make([]string, 0, len(publications))
	for _, p := range publications {
		ps = append(ps, string(p))
	}
	sort.Strings(ps)
	cs := make([]string, 0, len(consumptions))
	for _, c := range consumptions {
		cs = append(cs, string(c))
	}
	sort.Strings(cs)
	is := make([]string, 0, len(intentions))
	for _, i := range intentions {
		is = append(is, string(i))
	}
	sort.Strings(is)
	ts := append([]string(nil), tags...)
	sort.Strings(ts)
	b, err := json.Marshal(map[string]any{
		"query":        domain.SearchKey(query),
		"kinds":        ks,
		"publications": ps,
		"consumptions": cs,
		"unknown_cons": unknownConsumption,
		"intentions":   is,
		"tags":         ts,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func issueLibraryCursor(sort LibrarySort, limit int, qHash, key, id string) string {
	b, _ := json.Marshal(libraryCursorPayload{V: 1, Sort: string(sort), Limit: limit, QHash: qHash, Key: key, ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func parseLibraryCursor(raw string, sort LibrarySort, limit int, qHash string) (*libraryCursorPayload, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("the cursor is not decodable")
	}
	var c libraryCursorPayload
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("the cursor is not decodable")
	}
	if c.V != 1 {
		return nil, fmt.Errorf("the cursor version is not supported")
	}
	if c.Sort != string(sort) || c.Limit != limit || c.QHash != qHash {
		return nil, fmt.Errorf("the cursor does not match the query; start a new listing")
	}
	if c.ID == "" {
		return nil, fmt.Errorf("the cursor does not match the query; start a new listing")
	}
	return &c, nil
}

// SearchLibrary lists distinct works matching the filter as work cards,
// paginated with an opaque seek cursor. The total counts distinct filtered
// works before pagination.
func (s *Service) SearchLibrary(ctx context.Context, f LibraryFilter) (ListPayload, error) {
	base := MutationBase{}
	srt := f.Sort
	if srt == "" {
		srt = SortTitle
	}
	if !srt.Valid() {
		return ListPayload{}, invalidArgument("unknown sort "+string(srt), base)
	}
	limit := f.Limit
	if limit == 0 {
		limit = DefaultLibraryLimit
	}
	if limit < 1 || limit > MaxLibraryLimit {
		return ListPayload{}, invalidArgument(fmt.Sprintf("limit must be between 1 and %d", MaxLibraryLimit), base)
	}
	query := ""
	if f.Query != nil {
		if utf8.RuneCountInString(*f.Query) > MaxLibraryQueryRunes {
			return ListPayload{}, invalidArgument(fmt.Sprintf("query exceeds %d characters", MaxLibraryQueryRunes), base)
		}
		query = *f.Query
	}
	for _, k := range f.Kinds {
		if !k.Valid() {
			return ListPayload{}, invalidArgument("unknown kind "+string(k), base)
		}
	}
	for _, p := range f.Publications {
		if !p.Valid() {
			return ListPayload{}, invalidArgument("unknown publication "+string(p), base)
		}
	}
	for _, c := range f.Consumptions {
		if !c.Valid() {
			return ListPayload{}, invalidArgument("unknown consumption "+string(c), base)
		}
	}
	for _, i := range f.Intentions {
		if !i.Valid() {
			return ListPayload{}, invalidArgument("unknown intention "+string(i), base)
		}
	}
	tagNames := make([]string, 0, len(f.Tags))
	for _, t := range f.Tags {
		name, err := store.NormalizeTagName(t)
		if err != nil {
			return ListPayload{}, invalidArgument(err.Error(), base)
		}
		tagNames = append(tagNames, name)
	}
	qHash := filterFingerprint(query, f.Kinds, f.Publications, f.Consumptions, f.UnknownConsumption, f.Intentions, tagNames)
	var afterKey, afterID string
	if f.Cursor != nil {
		c, err := parseLibraryCursor(*f.Cursor, srt, limit, qHash)
		if err != nil {
			return ListPayload{}, invalidArgument(err.Error(), base)
		}
		afterKey, afterID = c.Key, c.ID
	}
	cards, total, err := s.db.SearchLibrary(ctx, store.LibraryQuery{
		Query:              query,
		Kinds:              f.Kinds,
		Publications:       f.Publications,
		Consumptions:       f.Consumptions,
		UnknownConsumption: f.UnknownConsumption,
		Intentions:         f.Intentions,
		Tags:               tagNames,
		Sort:               storeSort(srt),
		Limit:              limit + 1,
		AfterKey:           afterKey,
		AfterID:            afterID,
	})
	if err != nil {
		return ListPayload{}, mapStoreError(err, base)
	}
	var next *string
	if len(cards) > limit {
		// A probe row beyond the page proves more rows exist; the cursor
		// seeks after the last served row, never after the probe, so no
		// work is skipped at a page boundary.
		last := cards[limit-1]
		next = strPtr(issueLibraryCursor(srt, limit, qHash, last.SortKey, last.Work.ID))
		cards = cards[:limit]
	}
	items := make([]any, 0, len(cards))
	for _, c := range cards {
		items = append(items, toAppCard(c))
	}
	return ListPayload{Items: items, NextCursor: next, Total: total}, nil
}

func storeSort(s LibrarySort) store.SortMode {
	switch s {
	case SortCreatedAt:
		return store.SortCreatedAt
	case SortUpdatedAt:
		return store.SortUpdatedAt
	default:
		return store.SortTitle
	}
}

func toAppCard(c store.WorkCard) WorkCard {
	titles := make([]Title, 0, len(c.Titles))
	for _, t := range c.Titles {
		titles = append(titles, Title{ID: t.ID, Text: t.Text, Language: t.Language, Kind: t.Kind, Preferred: t.Preferred, Provenance: t.Provenance})
	}
	editions := make([]EditionSummary, 0, len(c.Editions))
	for _, e := range c.Editions {
		editions = append(editions, EditionSummary{ID: e.ID, Kind: e.Kind, Language: e.Language, Revision: e.Revision})
	}
	matched := c.MatchedEditionIDs
	if matched == nil {
		matched = []string{}
	}
	return WorkCard{
		WorkID:            c.ID,
		Revision:          c.Revision,
		CreatedAt:         c.CreatedAt,
		UpdatedAt:         c.UpdatedAt,
		ArchivedAt:        c.ArchivedAt,
		Titles:            titles,
		Editions:          editions,
		MatchedEditionIDs: matched,
	}
}

// GetWork returns the work detail view; a missing work is a stable
// not-found error.
func (s *Service) GetWork(ctx context.Context, workID string) (WorkView, error) {
	base := MutationBase{}
	if strings.TrimSpace(workID) == "" {
		return WorkView{}, invalidArgument("work id is required", base)
	}
	w, err := s.db.GetWork(ctx, workID)
	if err != nil {
		return WorkView{}, mapStoreError(err, base)
	}
	editions, err := s.db.ListEditions(ctx, workID)
	if err != nil {
		return WorkView{}, mapStoreError(err, base)
	}
	return workViewFrom(w, editions), nil
}

func workViewFrom(w store.Work, editions []store.Edition) WorkView {
	view := WorkView{
		ID:         w.ID,
		Revision:   w.Revision,
		CreatedAt:  w.CreatedAt,
		UpdatedAt:  w.UpdatedAt,
		ArchivedAt: w.ArchivedAt,
		Titles:     []Title{},
		Editions:   []EditionSummary{},
	}
	for _, t := range w.Titles {
		view.Titles = append(view.Titles, Title{ID: t.ID, Text: t.Text, Language: t.Language, Kind: t.Kind, Preferred: t.Preferred, Provenance: t.Provenance})
	}
	for _, e := range editions {
		view.Editions = append(view.Editions, EditionSummary{ID: e.ID, Kind: e.Kind, Language: e.Language, Revision: e.Revision})
	}
	return view
}

// GetEdition returns the edition detail view. Dimensions that later tasks
// have not landed yet are empty collections or null, never invented values.
func (s *Service) GetEdition(ctx context.Context, editionID string) (EditionView, error) {
	base := MutationBase{}
	if strings.TrimSpace(editionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", base)
	}
	e, err := s.db.GetEdition(ctx, editionID)
	if err != nil {
		return EditionView{}, mapStoreError(err, base)
	}
	return editionViewFrom(e), nil
}

// consumptionViewFrom presents the current consumption observation; an
// explicit retraction to unknown (like the absence of any observation) is
// the absence of a consumption state, never an invented value.
func consumptionViewFrom(o *store.ConsumptionObservation) *ConsumptionState {
	if o == nil || o.Value == store.ConsumptionUnknown {
		return nil
	}
	return &ConsumptionState{
		Value:        domain.Consumption(o.Value),
		Reason:       o.Reason,
		ObservedAt:   o.ObservedAt,
		RecordedAt:   o.RecordedAt,
		Actor:        Actor{Principal: o.Actor.Principal, Origin: domain.ActorOrigin(o.Actor.Origin), Identity: o.Actor.Identity},
		SupersedesID: o.SupersedesID,
	}
}

// intentionViewFrom presents the current intention observation; an explicit
// retraction to none (like the absence of any observation) is the absence
// of a recorded intention, never an invented value.
func intentionViewFrom(o *store.IntentionObservation) *IntentionState {
	if o == nil || o.Value == store.IntentionNone {
		return nil
	}
	return &IntentionState{
		Value:        domain.Intention(o.Value),
		Reason:       o.Reason,
		ObservedAt:   o.ObservedAt,
		RecordedAt:   o.RecordedAt,
		Actor:        Actor{Principal: o.Actor.Principal, Origin: domain.ActorOrigin(o.Actor.Origin), Identity: o.Actor.Identity},
		SupersedesID: o.SupersedesID,
	}
}

// tagsViewFrom presents the active tag names as a non-nil collection.
func tagsViewFrom(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// notesViewFrom presents the target's active notes, each at its latest
// revision: a retracted note is absent from the view, never rendered as a
// value, while its revisions stay in history.
func notesViewFrom(notes []store.NoteInfo) []Note {
	view := []Note{}
	for _, n := range notes {
		view = append(view, Note{
			ID:         n.ID,
			TargetID:   n.TargetID,
			Text:       n.Text,
			Spoiler:    n.Spoiler,
			Quote:      n.Quote,
			Location:   n.Location,
			Revision:   n.Revision,
			RecordedAt: n.RecordedAt,
			Actor:      Actor{Principal: n.Actor.Principal, Origin: domain.ActorOrigin(n.Actor.Origin), Identity: n.Actor.Identity},
		})
	}
	return view
}

// TitleMutationRequest is one explicit title correction on a work or an
// edition: add, retire and re-point the preferred display title in one
// atomic mutation.
type TitleMutationRequest struct {
	MutationBase
	TargetID  string
	Add       []store.TitleInput
	Retire    []string
	Preferred *string
}

func validateTitleMutation(base MutationBase, targetID string, add []store.TitleInput, retire []string, preferred *string) error {
	if strings.TrimSpace(targetID) == "" {
		return fmt.Errorf("the target id is required")
	}
	if len(add) == 0 && len(retire) == 0 && preferred == nil {
		return fmt.Errorf("at least one title change is required")
	}
	for _, t := range add {
		if strings.TrimSpace(t.Text) == "" {
			return fmt.Errorf("title text is empty")
		}
		if !t.Kind.Valid() {
			return fmt.Errorf("unknown title kind %q", t.Kind)
		}
		if t.Language != nil && strings.TrimSpace(*t.Language) == "" {
			return fmt.Errorf("title language must be null or non-empty")
		}
		if strings.TrimSpace(t.Provenance) == "" {
			return fmt.Errorf("title provenance is required")
		}
	}
	for _, id := range retire {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("title id is empty")
		}
	}
	if preferred != nil {
		if strings.TrimSpace(*preferred) == "" {
			return fmt.Errorf("preferred title id is empty")
		}
		for _, id := range retire {
			if id == *preferred {
				return fmt.Errorf("the preferred title is being retired")
			}
		}
	}
	return nil
}

// CorrectWorkTitles applies one title correction to a work and returns the
// updated work view.
func (s *Service) CorrectWorkTitles(ctx context.Context, req TitleMutationRequest) (WorkView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return WorkView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return WorkView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if err := validateTitleMutation(req.MutationBase, req.TargetID, req.Add, req.Retire, req.Preferred); err != nil {
		return WorkView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return WorkView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayWorkView(ctx, entry, req.MutationBase, hash)
	}
	m := store.TitleMutation{Add: req.Add, Retire: req.Retire, Preferred: req.Preferred}
	w, err := s.db.ApplyWorkTitles(ctx, s.now(), req.TargetID, *req.ExpectedRevision, m, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "correct_work_titles",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleWorkView(ctx, req.MutationBase, hash, err)
	}
	editions, err := s.db.ListEditions(ctx, w.ID)
	if err != nil {
		return WorkView{}, mapStoreError(err, req.MutationBase)
	}
	return workViewFrom(w, editions), nil
}

// CorrectEditionTitles applies one title correction to an edition and
// returns the updated edition view.
func (s *Service) CorrectEditionTitles(ctx context.Context, req TitleMutationRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if err := validateTitleMutation(req.MutationBase, req.TargetID, req.Add, req.Retire, req.Preferred); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	m := store.TitleMutation{Add: req.Add, Retire: req.Retire, Preferred: req.Preferred}
	e, err := s.db.ApplyEditionTitles(ctx, s.now(), req.TargetID, *req.ExpectedRevision, m, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "correct_edition_titles",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

// UpdateEditionRequest is one edition metadata correction: kind, language
// and other label; only supplied fields change.
type UpdateEditionRequest struct {
	MutationBase
	EditionID  string
	Kind       *domain.EditionKind
	Language   *string
	OtherLabel *string
}

// UpdateEdition applies one edition metadata correction and returns the
// updated edition view. A same-value no-op succeeds without incrementing
// the revision.
func (s *Service) UpdateEdition(ctx context.Context, req UpdateEditionRequest) (EditionView, error) {
	if err := requireMutationBase(req.MutationBase); err != nil {
		return EditionView{}, invalidArgument(err.Error(), req.MutationBase)
	}
	if req.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", req.MutationBase)
	}
	if strings.TrimSpace(req.EditionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", req.MutationBase)
	}
	if req.Kind == nil && req.Language == nil && req.OtherLabel == nil {
		return EditionView{}, invalidArgument("at least one metadata field is required", req.MutationBase)
	}
	if req.Kind != nil && !req.Kind.Valid() {
		return EditionView{}, invalidArgument("unknown kind "+string(*req.Kind), req.MutationBase)
	}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, req.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, req.MutationBase)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, req.MutationBase, hash)
	}
	e, err := s.db.UpdateEditionMeta(ctx, s.now(), req.EditionID, *req.ExpectedRevision, req.Kind, req.Language, req.OtherLabel, &store.JournalInput{
		RequestID:   req.RequestID,
		Operation:   "update_edition",
		RequestHash: hash,
		Actor:       req.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

// ArchiveWork archives or unarchives a work; titles, editions and history
// are preserved.
func (s *Service) ArchiveWork(ctx context.Context, base MutationBase, workID string, archived bool) (WorkView, error) {
	if err := requireMutationBase(base); err != nil {
		return WorkView{}, invalidArgument(err.Error(), base)
	}
	if base.ExpectedRevision == nil {
		return WorkView{}, invalidArgument("expected_revision is required for existing-entity mutations", base)
	}
	if strings.TrimSpace(workID) == "" {
		return WorkView{}, invalidArgument("work id is required", base)
	}
	req := struct {
		MutationBase
		WorkID   string `json:"work_id"`
		Archived bool   `json:"archived"`
	}{MutationBase: base, WorkID: workID, Archived: archived}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, base.RequestID); err != nil {
		return WorkView{}, mapStoreError(err, base)
	} else if entry != nil {
		return s.replayWorkView(ctx, entry, base, hash)
	}
	w, err := s.db.SetWorkArchived(ctx, s.now(), workID, *base.ExpectedRevision, archived, &store.JournalInput{
		RequestID:   base.RequestID,
		Operation:   "archive_work",
		RequestHash: hash,
		Actor:       base.Actor.Principal,
	})
	if err != nil {
		return s.settleWorkView(ctx, req.MutationBase, hash, err)
	}
	editions, err := s.db.ListEditions(ctx, w.ID)
	if err != nil {
		return WorkView{}, mapStoreError(err, base)
	}
	return workViewFrom(w, editions), nil
}

// ArchiveEdition archives or unarchives an edition; titles, observations
// and history are preserved.
func (s *Service) ArchiveEdition(ctx context.Context, base MutationBase, editionID string, archived bool) (EditionView, error) {
	if err := requireMutationBase(base); err != nil {
		return EditionView{}, invalidArgument(err.Error(), base)
	}
	if base.ExpectedRevision == nil {
		return EditionView{}, invalidArgument("expected_revision is required for existing-entity mutations", base)
	}
	if strings.TrimSpace(editionID) == "" {
		return EditionView{}, invalidArgument("edition id is required", base)
	}
	req := struct {
		MutationBase
		EditionID string `json:"edition_id"`
		Archived  bool   `json:"archived"`
	}{MutationBase: base, EditionID: editionID, Archived: archived}
	hash := payloadHash(req)
	if entry, err := s.db.LookupJournal(ctx, base.RequestID); err != nil {
		return EditionView{}, mapStoreError(err, base)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, base, hash)
	}
	e, err := s.db.SetEditionArchived(ctx, s.now(), editionID, *base.ExpectedRevision, archived, &store.JournalInput{
		RequestID:   base.RequestID,
		Operation:   "archive_edition",
		RequestHash: hash,
		Actor:       base.Actor.Principal,
	})
	if err != nil {
		return s.settleEditionView(ctx, req.MutationBase, hash, err)
	}
	return editionViewFrom(e), nil
}

func editionViewFrom(e store.Edition) EditionView {
	view := EditionView{
		ID:         e.ID,
		WorkID:     e.WorkID,
		Kind:       e.Kind,
		Language:   e.Language,
		OtherLabel: e.OtherLabel,
		Revision:   e.Revision,
		Titles:     []Title{},
		Progress:   ProgressByUnit{},
		Tags:       []string{},
		References: []ExternalReference{},
		Notes:      []Note{},
	}
	for _, t := range e.Titles {
		view.Titles = append(view.Titles, Title{ID: t.ID, Text: t.Text, Language: t.Language, Kind: t.Kind, Preferred: t.Preferred, Provenance: t.Provenance})
	}
	if e.Publication != nil {
		view.Publication = &PublicationState{
			Value:        e.Publication.Value,
			Source:       e.Publication.Source,
			ObservedAt:   e.Publication.ObservedAt,
			RecordedAt:   e.Publication.RecordedAt,
			Provenance:   e.Publication.Provenance,
			SupersedesID: e.Publication.SupersedesID,
		}
	}
	view.Consumption = consumptionViewFrom(e.Consumption)
	view.Intention = intentionViewFrom(e.Intention)
	view.Tags = tagsViewFrom(e.Tags)
	view.Notes = notesViewFrom(e.Notes)
	view.Progress = progressViewFrom(e.Progress)
	return view
}

// requireMutationBase checks the mutation identity fields.
func requireMutationBase(base MutationBase) error {
	if strings.TrimSpace(base.RequestID) == "" {
		return fmt.Errorf("request_id is required")
	}
	if strings.TrimSpace(base.Actor.Principal) == "" {
		return fmt.Errorf("actor.principal is required")
	}
	return nil
}

// payloadHash is the canonical request hash of one mutation: the stable
// JSON of the whole request, mutation base included.
func payloadHash(req any) string {
	b, err := json.Marshal(req)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// entitySummary decodes the journal summary of one entity mutation.
type entitySummary struct {
	WorkID    string `json:"work_id"`
	EditionID string `json:"edition_id"`
}

func decodeEntitySummary(entry *store.JournalEntry, base MutationBase) (entitySummary, *AppError) {
	var sum entitySummary
	if err := json.Unmarshal([]byte(entry.ResultJSON), &sum); err != nil {
		return sum, &AppError{Code: ErrStorageError, Message: "stored mutation result is corrupt", RequestID: base.RequestID, Actor: base.Actor.Principal}
	}
	return sum, nil
}

// replayWorkView settles a same-key work mutation against the journal: a
// matching hash re-derives the stored work view, a mismatched hash is a
// key-collision conflict, and a corrupt row is a storage error.
func (s *Service) replayWorkView(ctx context.Context, entry *store.JournalEntry, base MutationBase, hash string) (WorkView, error) {
	if entry.RequestHash != hash {
		return WorkView{}, conflict("the idempotency key was already used with a different payload", base)
	}
	sum, aerr := decodeEntitySummary(entry, base)
	if aerr != nil {
		return WorkView{}, aerr
	}
	return s.GetWork(ctx, sum.WorkID)
}

func (s *Service) replayEditionView(ctx context.Context, entry *store.JournalEntry, base MutationBase, hash string) (EditionView, error) {
	if entry.RequestHash != hash {
		return EditionView{}, conflict("the idempotency key was already used with a different payload", base)
	}
	sum, aerr := decodeEntitySummary(entry, base)
	if aerr != nil {
		return EditionView{}, aerr
	}
	return s.GetEdition(ctx, sum.EditionID)
}

// settleWorkView consults the journal after a failed work mutation: a
// concurrent commit of the same key replays its stored view, a different
// payload on the same key conflicts, and anything else maps the error.
func (s *Service) settleWorkView(ctx context.Context, base MutationBase, hash string, err error) (WorkView, error) {
	if entry, lerr := s.db.LookupJournal(ctx, base.RequestID); lerr != nil {
		return WorkView{}, mapStoreError(lerr, base)
	} else if entry != nil {
		return s.replayWorkView(ctx, entry, base, hash)
	}
	return WorkView{}, mapStoreError(err, base)
}

func (s *Service) settleEditionView(ctx context.Context, base MutationBase, hash string, err error) (EditionView, error) {
	if entry, lerr := s.db.LookupJournal(ctx, base.RequestID); lerr != nil {
		return EditionView{}, mapStoreError(lerr, base)
	} else if entry != nil {
		return s.replayEditionView(ctx, entry, base, hash)
	}
	return EditionView{}, mapStoreError(err, base)
}
