package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestSetAndGet(t *testing.T) {
	defer Set(Ru)
	for _, l := range Order {
		Set(l)
		if Get() != l {
			t.Fatalf("Set(%s) → Get() = %s", l, Get())
		}
	}
	Set("nonsense")
	if Get() != Ru && Get() != En && Get() != De {
		t.Fatalf("мусорный язык не должен ломать состояние: %s", Get())
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]Lang{
		"en": En, "EN": En, "english": En,
		"de": De, "Deutsch": De, "german": De,
		"ru": Ru, "русский": Ru,
		"xx": "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, ждём %q", in, got, want)
		}
	}
}

func TestCycleCoversThreeLanguages(t *testing.T) {
	seen := map[Lang]bool{}
	l := Ru
	for i := 0; i < 3; i++ {
		seen[l] = true
		l = l.Next()
	}
	if len(seen) != 3 {
		t.Fatalf("/lang должен перебирать все три языка, покрыто %d", len(seen))
	}
}

func TestTranslationChangesTheText(t *testing.T) {
	defer Set(Ru)
	ru := "Память отключена"
	Set(Ru)
	if got := S(ru); got != ru {
		t.Fatalf("на русском ожидался исходник, получено %q", got)
	}
	Set(En)
	en := S(ru)
	if en == ru || en == "" {
		t.Fatalf("английский перевод пустой или совпал: %q", en)
	}
	Set(De)
	de := S(ru)
	if de == ru || de == "" || de == en {
		t.Fatalf("немецкий перевод пустой или совпал с английским: %q", de)
	}
}

func TestFormatVerbsMatchAcrossLanguages(t *testing.T) {
	for ru, msg := range messages {
		if msg.En == "" || msg.De == "" {
			t.Errorf("нет перевода для %q", ru)
			continue
		}
		if v, e := Verbs(ru), Verbs(msg.En); v != e {
			t.Errorf("английский перевод %q: аргументы %q вместо %q", ru, e, v)
		}
		if v, d := Verbs(ru), Verbs(msg.De); v != d {
			t.Errorf("немецкий перевод %q: аргументы %q вместо %q", ru, d, v)
		}
	}
}

func TestUntranslatedStringFallsBackToRussian(t *testing.T) {
	Set(En)
	if got := S("строка которой нет в каталоге"); got != "строка которой нет в каталоге" {
		t.Fatalf("неизвестная строка должна показываться как есть, получено %q", got)
	}
}

func TestFSubstitutesArguments(t *testing.T) {
	Set(En)
	got := F("Навык сохранён: %q (/skills чтобы посмотреть)", "миграция")
	if !strings.Contains(got, "миграция") {
		t.Fatalf("аргумент не подставился: %q", got)
	}
}

// TestEveryUserFacingStringIsTranslated keeps the catalog honest: any Russian
// literal added to the TUI or the CLI must get an English and German version.
func TestEveryUserFacingStringIsTranslated(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	rx := regexp.MustCompile(`"((?:[^"\\\n]|\\.)*[А-Яа-яЁё][^"\\\n]*)"`)
	found := map[string]string{}
	for _, dir := range []string{"internal/tui", "cmd/allan"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, dir, name))
			if err != nil {
				t.Fatal(err)
			}
			src := string(raw)
			_, file, _, _ := runtime.Caller(0)
			_ = file
			for _, m := range rx.FindAllStringSubmatch(src, -1) {
				key := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\\`, `\`).Replace(m[1])
				if _, ok := found[key]; !ok {
					found[key] = dir + "/" + name
				}
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("сканер ничего не нашёл — он сломан")
	}
	var missing []string
	for key, where := range found {
		if !Has(key) {
			missing = append(missing, where+": "+key)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("строки без перевода (добавьте их в internal/i18n/gen.py):\n%s",
			strings.Join(missing, "\n"))
	}
}
