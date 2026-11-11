package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// EditionOtherLabelMax bounds the descriptive label required for kind other.
const EditionOtherLabelMax = 64

// MaxFutureObservation bounds how far ahead of server time an observation's
// observed_at may be; later instants are rejected, not clamped.
const MaxFutureObservation = 5 * time.Minute

// ValidateEditionAttrs checks the edition identity attributes: kind is a
// bounded value, language is null when unknown and never empty, and the
// descriptive other-label is required for kind other and forbidden for all
// other kinds. A translation keeps its medium: the kind is never derived
// from language or any other field.
func ValidateEditionAttrs(kind EditionKind, language, otherLabel *string) error {
	if !kind.Valid() {
		return fmt.Errorf("domain: unknown edition kind %q", kind)
	}
	if language != nil && strings.TrimSpace(*language) == "" {
		return fmt.Errorf("domain: edition language must be null or non-empty")
	}
	if (kind == KindOther) != (otherLabel != nil) {
		return fmt.Errorf("domain: kind %q requires a descriptive label; other kinds do not take one", kind)
	}
	if otherLabel != nil {
		label := *otherLabel
		if strings.TrimSpace(label) == "" {
			return fmt.Errorf("domain: edition other label is empty")
		}
		if utf8.RuneCountInString(label) > EditionOtherLabelMax {
			return fmt.Errorf("domain: edition other label exceeds %d characters", EditionOtherLabelMax)
		}
	}
	return nil
}

// ValidatePublicationAttrs checks one publication observation: bounded
// value, required provenance, a required source for every known claim
// (a manual assertion is a source), and observed_at within the allowed
// future window. A zero observed_at defaults to now.
func ValidatePublicationAttrs(value Publication, source, provenance string, observedAt, now time.Time) error {
	if !value.Valid() {
		return fmt.Errorf("domain: unknown publication value %q", value)
	}
	if strings.TrimSpace(provenance) == "" {
		return fmt.Errorf("domain: publication provenance is required")
	}
	if value != PublicationUnknown && strings.TrimSpace(source) == "" {
		return fmt.Errorf("domain: a known publication claim requires a source")
	}
	if observedAt.IsZero() {
		observedAt = now
	}
	if observedAt.After(now.Add(MaxFutureObservation)) {
		return fmt.Errorf("domain: observed_at exceeds server time by more than %s", MaxFutureObservation)
	}
	return nil
}
