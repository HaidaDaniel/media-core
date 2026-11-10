// Package domain defines the bounded value types shared by all adapters.
package domain

import (
	"encoding/json"
	"fmt"
)

// parseEnum decodes a bounded enum from JSON, rejecting null and unknown
// values so invalid enums never reach application logic.
func parseEnum[T ~string](b []byte, name string, valid func(T) bool) (T, error) {
	if string(b) == "null" {
		return T(""), fmt.Errorf("domain: %s: unexpected null", name)
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return T(""), err
	}
	v := T(s)
	if !valid(v) {
		return T(""), fmt.Errorf("domain: %s: invalid value %q", name, s)
	}
	return v, nil
}

// EditionKind is the bounded set of publication media.
type EditionKind string

// Edition kinds. A Russian manga translation remains manga; translation is
// a relation, not a kind.
const (
	KindWebNovel    EditionKind = "web_novel"
	KindLightNovel  EditionKind = "light_novel"
	KindBook        EditionKind = "book"
	KindManga       EditionKind = "manga"
	KindManhwa      EditionKind = "manhwa"
	KindManhua      EditionKind = "manhua"
	KindAnime       EditionKind = "anime"
	KindMovie       EditionKind = "movie"
	KindTV          EditionKind = "tv"
	KindGame        EditionKind = "game"
	KindVisualNovel EditionKind = "visual_novel"
	KindOther       EditionKind = "other"
	KindUnknown     EditionKind = "unknown"
)

// Valid reports whether the kind is a defined v1 value.
func (k EditionKind) Valid() bool {
	switch k {
	case KindWebNovel, KindLightNovel, KindBook, KindManga, KindManhwa, KindManhua,
		KindAnime, KindMovie, KindTV, KindGame, KindVisualNovel, KindOther, KindUnknown:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown kinds and null.
func (k *EditionKind) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "edition kind", func(v EditionKind) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*k = v
	return nil
}

// TitleKind classifies a title row.
type TitleKind string

const (
	TitleOriginal  TitleKind = "original"
	TitleLocalized TitleKind = "localized"
	TitleRomanized TitleKind = "romanized"
	TitleAlias     TitleKind = "alias"
)

// Valid reports whether the kind is a defined v1 value.
func (k TitleKind) Valid() bool {
	switch k {
	case TitleOriginal, TitleLocalized, TitleRomanized, TitleAlias:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown kinds and null.
func (k *TitleKind) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "title kind", func(v TitleKind) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*k = v
	return nil
}

// Consumption is a personal consumption state value.
type Consumption string

const (
	ConsumptionNotStarted Consumption = "not_started"
	ConsumptionInProgress Consumption = "in_progress"
	ConsumptionPaused     Consumption = "paused"
	ConsumptionCompleted  Consumption = "completed"
	ConsumptionDropped    Consumption = "dropped"
)

// Valid reports whether the value is a defined v1 state.
func (c Consumption) Valid() bool {
	switch c {
	case ConsumptionNotStarted, ConsumptionInProgress, ConsumptionPaused,
		ConsumptionCompleted, ConsumptionDropped:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown values and null (unknown is the absence of
// a state object, not a null value).
func (c *Consumption) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "consumption", func(v Consumption) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*c = v
	return nil
}

// Intention is a personal intention value; none means no recorded
// intention, not dislike.
type Intention string

const (
	IntentionNone          Intention = "none"
	IntentionInterested    Intention = "interested"
	IntentionSaved         Intention = "saved"
	IntentionPlanned       Intention = "planned"
	IntentionNotInterested Intention = "not_interested"
)

// Valid reports whether the value is a defined v1 intention.
func (i Intention) Valid() bool {
	switch i {
	case IntentionNone, IntentionInterested, IntentionSaved, IntentionPlanned, IntentionNotInterested:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown values and null.
func (i *Intention) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "intention", func(v Intention) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*i = v
	return nil
}

// Publication is an edition publication state value.
type Publication string

const (
	PublicationUnknown   Publication = "unknown"
	PublicationOngoing   Publication = "ongoing"
	PublicationCompleted Publication = "completed"
	PublicationHiatus    Publication = "hiatus"
	PublicationCancelled Publication = "cancelled"
)

// Valid reports whether the value is a defined v1 publication state.
func (p Publication) Valid() bool {
	switch p {
	case PublicationUnknown, PublicationOngoing, PublicationCompleted,
		PublicationHiatus, PublicationCancelled:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown values and null.
func (p *Publication) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "publication", func(v Publication) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*p = v
	return nil
}

// ProgressUnit is the bounded set of progress units.
type ProgressUnit string

const (
	UnitChapter ProgressUnit = "chapter"
	UnitVolume  ProgressUnit = "volume"
	UnitEpisode ProgressUnit = "episode"
	UnitPage    ProgressUnit = "page"
	UnitPercent ProgressUnit = "percent"
)

// Valid reports whether the unit is a defined v1 unit.
func (u ProgressUnit) Valid() bool {
	switch u {
	case UnitChapter, UnitVolume, UnitEpisode, UnitPage, UnitPercent:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown units and null.
func (u *ProgressUnit) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "progress unit", func(v ProgressUnit) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*u = v
	return nil
}

// ReferenceRole classifies an external reference.
type ReferenceRole string

const (
	RolePublication        ReferenceRole = "publication"
	RoleMetadata           ReferenceRole = "metadata"
	RoleTranslation        ReferenceRole = "translation"
	RoleReleaseWatcher     ReferenceRole = "release_watcher"
	RoleTranslationProject ReferenceRole = "translation_project"
	RoleContent            ReferenceRole = "content"
)

// Valid reports whether the role is a defined v1 role.
func (r ReferenceRole) Valid() bool {
	switch r {
	case RolePublication, RoleMetadata, RoleTranslation, RoleReleaseWatcher,
		RoleTranslationProject, RoleContent:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown roles and null.
func (r *ReferenceRole) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "reference role", func(v ReferenceRole) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*r = v
	return nil
}

// OriginKind classifies an origin claim.
type OriginKind string

const (
	OriginOriginal      OriginKind = "original"
	OriginAnimeOriginal OriginKind = "anime_original"
	OriginUnknown       OriginKind = "unknown"
)

// Valid reports whether the kind is a defined v1 origin kind.
func (k OriginKind) Valid() bool {
	switch k {
	case OriginOriginal, OriginAnimeOriginal, OriginUnknown:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown kinds and null.
func (k *OriginKind) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "origin kind", func(v OriginKind) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*k = v
	return nil
}

// ClaimState is the lifecycle state of a relation or origin claim.
type ClaimState string

const (
	ClaimProposed  ClaimState = "proposed"
	ClaimConfirmed ClaimState = "confirmed"
	ClaimRevoked   ClaimState = "revoked"
)

// Valid reports whether the state is a defined v1 state.
func (s ClaimState) Valid() bool {
	switch s {
	case ClaimProposed, ClaimConfirmed, ClaimRevoked:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown states and null.
func (s *ClaimState) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "claim state", func(v ClaimState) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// RelationType is a typed media relation.
type RelationType string

const (
	RelationAdaptationOf       RelationType = "adaptation_of"
	RelationTranslationOf      RelationType = "translation_of"
	RelationAlternativeVersion RelationType = "alternative_version"
	RelationSameUnderlyingWork RelationType = "same_underlying_work"
	RelationSequelOf           RelationType = "sequel_of"
	RelationSideStoryOf        RelationType = "side_story_of"
	RelationRelated            RelationType = "related"
)

// Valid reports whether the type is a defined v1 relation.
func (r RelationType) Valid() bool {
	switch r {
	case RelationAdaptationOf, RelationTranslationOf, RelationAlternativeVersion,
		RelationSameUnderlyingWork, RelationSequelOf, RelationSideStoryOf, RelationRelated:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown types and null.
func (r *RelationType) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "relation type", func(v RelationType) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*r = v
	return nil
}

// ActorOrigin is the asserted origin class of an actor.
type ActorOrigin string

const (
	OriginUser   ActorOrigin = "user"
	OriginAgent  ActorOrigin = "agent"
	OriginImport ActorOrigin = "import"
)

// Valid reports whether the origin is a defined v1 class.
func (o ActorOrigin) Valid() bool {
	switch o {
	case OriginUser, OriginAgent, OriginImport:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown origins and null.
func (o *ActorOrigin) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "actor origin", func(v ActorOrigin) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*o = v
	return nil
}
