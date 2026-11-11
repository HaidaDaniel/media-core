package domain

import (
	"crypto/rand"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// caseFolder applies Unicode full case folding (CaseFolding.txt type F),
// which maps multi-rune sequences such as ss for eszett and keeps diacritics,
// kana and punctuation untouched.
var caseFolder = cases.Fold()

// NewUUID returns a random UUID v4 string. IDs are opaque random UUIDs per
// the application contract.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("domain: generate UUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// SearchKey computes the deterministic title search key: Unicode NFKC, full
// Unicode case fold, then trim and collapse of Unicode whitespace. Original
// display text is preserved separately; the key is for matching only.
// Diacritics, kana, kanji, punctuation and yo are preserved: there is no
// transliteration and no yo-to-e equivalence. Romaji and translated aliases
// are separate title rows, not implicit expansions.
func SearchKey(text string) string {
	s := norm.NFKC.String(text)
	s = caseFolder.String(s)
	return collapseWhitespace(s)
}

// collapseWhitespace trims the ends and collapses every run of Unicode
// whitespace to a single ASCII space; the separator is written lazily so a
// trailing run never leaves a space behind.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pending := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			pending = true
			continue
		}
		if pending && b.Len() > 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
		pending = false
	}
	return b.String()
}
