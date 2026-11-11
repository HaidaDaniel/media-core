package domain

import "testing"

// TestNormalizeSearchKeyNFKC pins compatibility normalization: fullwidth
// forms and ligatures fold to their canonical equivalents before matching.
func TestNormalizeSearchKeyNFKC(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ＡＢＣ ｄｅｆ", "abc def"},
		{"ﬁn ﬂex", "fin flex"},
		{"x²", "x2"},
		{"\u212Aelvin", "kelvin"}, // compatibility Kelvin sign normalizes to K
	}
	for _, c := range cases {
		if got := SearchKey(c.in); got != c.want {
			t.Errorf("SearchKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeSearchKeyFullCaseFold pins full Unicode case folding, which
// includes multi-rune mappings, while diacritics stay intact.
func TestNormalizeSearchKeyFullCaseFold(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Straße", "strasse"},
		{"STRASSE", "strasse"},
		{"ΣΙΓΜΑ", "σιγμα"},
		{"İstanbul", "i\u0307stanbul"},
		{"Café", "café"},
		{"ЁЛКА", "ёлка"}, // no yo-to-e equivalence
		{"Ёлка", "ёлка"},
	}
	for _, c := range cases {
		if got := SearchKey(c.in); got != c.want {
			t.Errorf("SearchKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeSearchKeyWhitespace pins trim and collapse of Unicode
// whitespace runs to a single ASCII space.
func TestNormalizeSearchKeyWhitespace(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"  hello\u00a0world  ", "hello world"},
		{"hello\u3000world", "hello world"},
		{"a\u2003b\u202fc", "a b c"},
		{"\n\t lead", "lead"},
		{"trail \n\t", "trail"},
		{"no-space", "no-space"},
	}
	for _, c := range cases {
		if got := SearchKey(c.in); got != c.want {
			t.Errorf("SearchKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeSearchKeyKanaKanji pins that kana and kanji are preserved
// exactly: no romanization is ever implicit.
func TestNormalizeSearchKeyKanaKanji(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"進撃の巨人", "進撃の巨人"},
		{"Shingeki no Kyojin", "shingeki no kyojin"},
		{"アタックオンタイタン", "アタックオンタイタン"},
		{"Атака Титанов", "атака титанов"},
	}
	for _, c := range cases {
		if got := SearchKey(c.in); got != c.want {
			t.Errorf("SearchKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeSearchKeyEmpty(t *testing.T) {
	if got := SearchKey("   "); got != "" {
		t.Errorf("SearchKey(whitespace) = %q, want empty", got)
	}
}

func TestNewUUID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id, err := NewUUID()
		if err != nil {
			t.Fatalf("NewUUID: %v", err)
		}
		if len(id) != 36 || id[14] != '4' {
			t.Fatalf("NewUUID() = %q, want a UUID v4 string", id)
		}
		if seen[id] {
			t.Fatalf("NewUUID repeated: %q", id)
		}
		seen[id] = true
	}
}
