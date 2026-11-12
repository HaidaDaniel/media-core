package domain

// RelationScope is the scope of a relation endpoint pair: both endpoints
// are works or both are editions; a mixed work-edition link is invalid.
type RelationScope string

const (
	ScopeWork    RelationScope = "work"
	ScopeEdition RelationScope = "edition"
)

// Valid reports whether the scope is a defined v1 scope.
func (s RelationScope) Valid() bool {
	switch s {
	case ScopeWork, ScopeEdition:
		return true
	}
	return false
}

// UnmarshalJSON rejects unknown scopes and null.
func (s *RelationScope) UnmarshalJSON(b []byte) error {
	v, err := parseEnum(b, "relation scope", func(v RelationScope) bool { return v.Valid() })
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// Scopes reports the scopes a relation type may bind. related is the only
// type valid in both scopes; the others bind exactly one.
func (t RelationType) Scopes() []RelationScope {
	switch t {
	case RelationAdaptationOf, RelationTranslationOf, RelationAlternativeVersion:
		return []RelationScope{ScopeEdition}
	case RelationSameUnderlyingWork, RelationSequelOf, RelationSideStoryOf:
		return []RelationScope{ScopeWork}
	case RelationRelated:
		return []RelationScope{ScopeWork, ScopeEdition}
	}
	return nil
}

// AllowsScope reports whether the relation type may bind the scope.
func (t RelationType) AllowsScope(s RelationScope) bool {
	for _, sc := range t.Scopes() {
		if sc == s {
			return true
		}
	}
	return false
}

// Symmetric reports whether the relation's endpoints are undirected and
// stored in canonical order, so (a,b) and (b,a) are one edge.
func (t RelationType) Symmetric() bool {
	switch t {
	case RelationAlternativeVersion, RelationSameUnderlyingWork, RelationRelated:
		return true
	}
	return false
}

// CycleGraph reports which directed confirmed graph the relation joins when
// confirmed: the combined adaptation_of + translation_of source graph, the
// sequel graph or the side-story graph. An empty result means the type is
// undirected and may form cycles.
func (t RelationType) CycleGraph() string {
	switch t {
	case RelationAdaptationOf, RelationTranslationOf:
		return "source"
	case RelationSequelOf:
		return "sequel"
	case RelationSideStoryOf:
		return "side_story"
	}
	return ""
}

// CanonicalEndpoints orders an endpoint pair for symmetric storage; the
// directed types keep the caller's direction.
func CanonicalEndpoints(a, b string) (string, string) {
	if a <= b {
		return a, b
	}
	return b, a
}
