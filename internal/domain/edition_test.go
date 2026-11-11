package domain

import (
	"strings"
	"testing"
	"time"
)

func editionStrPtr(s string) *string { return &s }

func TestEditionAttrsValidation(t *testing.T) {
	lang := "ru"
	label := "radio play"
	longLabel := strings.Repeat("a", EditionOtherLabelMax+1)
	cases := []struct {
		name       string
		kind       EditionKind
		language   *string
		otherLabel *string
		wantErr    bool
	}{
		{"manga no label", KindManga, &lang, nil, false},
		{"manga with label rejected", KindManga, &lang, &label, true},
		{"other without label", KindOther, nil, nil, true},
		{"other with label", KindOther, nil, &label, false},
		{"other with empty label", KindOther, nil, editionStrPtr("   "), true},
		{"other with oversized label", KindOther, nil, &longLabel, true},
		{"other at label limit", KindOther, nil, editionStrPtr(strings.Repeat("a", EditionOtherLabelMax)), false},
		{"unknown kind ok", KindUnknown, nil, nil, false},
		{"bogus kind", EditionKind("hologram"), nil, nil, true},
		{"empty language", KindManga, editionStrPtr("  "), nil, true},
		{"nil language", KindAnime, nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateEditionAttrs(c.kind, c.language, c.otherLabel)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateEditionAttrs(%q, %v, %v) err = %v, wantErr = %v", c.kind, c.language, c.otherLabel, err, c.wantErr)
			}
		})
	}
}

func TestPublicationAttrsValidation(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	source := "manual"
	cases := []struct {
		name       string
		value      Publication
		source     string
		provenance string
		observed   time.Time
		wantErr    bool
	}{
		{"known with source", PublicationOngoing, source, "manual", now, false},
		{"known without source", PublicationCompleted, "", "manual", now, true},
		{"unknown without source", PublicationUnknown, "", "manual", now, false},
		{"unknown with source", PublicationUnknown, source, "manual", now, false},
		{"empty provenance", PublicationUnknown, "", "  ", now, true},
		{"bogus value", Publication("abandoned"), source, "manual", now, true},
		{"zero observed defaults to now", PublicationOngoing, source, "manual", time.Time{}, false},
		{"past observed allowed", PublicationOngoing, source, "manual", now.Add(-time.Hour), false},
		{"observed at the limit", PublicationOngoing, source, "manual", now.Add(MaxFutureObservation), false},
		{"observed beyond the limit", PublicationOngoing, source, "manual", now.Add(MaxFutureObservation + time.Second), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePublicationAttrs(c.value, c.source, c.provenance, c.observed, now)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidatePublicationAttrs(%q, %q, %q) err = %v, wantErr = %v", c.value, c.source, c.provenance, err, c.wantErr)
			}
		})
	}
}
