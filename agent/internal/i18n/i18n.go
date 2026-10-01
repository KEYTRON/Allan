// Package i18n translates Allan's messages.
//
// Russian is the source language: every string is written in the code as it
// should read for a Russian user, and the catalog in catalog.go holds the
// English and German versions keyed by that source text. That way adding a
// message means writing it once — and the catalog test fails until somebody
// translates it.
//
// Supported languages: Russian (default), English, German. /lang switches at
// runtime, --lang picks one for a run, the tui.lang config key remembers it.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Lang is a supported interface language.
type Lang string

const (
	Ru Lang = "ru"
	En Lang = "en"
	De Lang = "de"
)

// Msg is one message in the non-Russian languages. A missing translation falls
// back to the Russian source instead of showing an empty line.
type Msg struct {
	En string
	De string
}

// Order is the cycle order for /lang.
var Order = []Lang{Ru, En, De}

var current atomic.Value // Lang

func init() {
	current.Store(Ru)
}

// Set switches the active language. Unknown values are ignored.
func Set(l Lang) {
	norm := Normalize(string(l))
	if norm == "" {
		return
	}
	current.Store(norm)
}

// Get returns the active language.
func Get() Lang {
	if l, ok := current.Load().(Lang); ok && l != "" {
		return l
	}
	return Ru
}

// Normalize maps user input (en, EN, english, Deutsch…) to a Lang.
func Normalize(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "en", "eng", "english":
		return En
	case "de", "ger", "deu", "german", "deutsch":
		return De
	case "ru", "rus", "russian", "русский", "рус":
		return Ru
	}
	return ""
}

// Detect picks a language from the environment (LANG/LC_ALL), falling back to
// Russian. It is only consulted when neither the config nor a flag says so.
func Detect() Lang {
	for _, key := range []string{"ALLAN_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			if l := Normalize(v); l != "" {
				return l
			}
			if i := strings.IndexAny(v, ".@"); i > 0 {
				if l := Normalize(v[:i]); l != "" {
					return l
				}
			}
		}
	}
	return Ru
}

// Label is the language name in that language, for the /lang picker.
func (l Lang) Label() string {
	switch Normalize(string(l)) {
	case En:
		return "English"
	case De:
		return "Deutsch"
	default:
		return "Русский"
	}
}

// Next cycles to the following language, which is what a bare /lang does.
func (l Lang) Next() Lang {
	norm := func(x Lang) Lang { return Normalize(string(x)) }
	cur := norm(l)
	for i, cand := range Order {
		if norm(cand) == cur {
			return Order[(i+1)%len(Order)]
		}
	}
	return Ru
}

// S translates a static message written in Russian.
func S(ru string) string {
	msg, ok := messages[ru]
	if !ok {
		return ru
	}
	switch Get() {
	case En:
		return msg.En
	case De:
		return msg.De
	}
	return ru
}

// F translates a formatted message. The format verbs must be the same in every
// language and in the same order, otherwise the translation is ignored.
func F(ruFmt string, args ...any) string {
	return fmt.Sprintf(S(ruFmt), args...)
}

// Has reports whether a string is in the catalog (used by tests).
func Has(ru string) bool {
	_, ok := messages[ru]
	return ok
}

// Verbs returns the format verbs of a format string, so tests can check that a
// translation takes the same arguments as its source.
func Verbs(s string) string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		j := i + 1
		for j < len(s) && strings.ContainsRune("#0+- ", rune(s[j])) {
			j++
		}
		if j < len(s) {
			out = append(out, string(s[j]))
			i = j
		}
	}
	return strings.Join(out, "")
}
