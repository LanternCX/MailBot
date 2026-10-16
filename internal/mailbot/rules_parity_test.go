package mailbot

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyRulesUnicodeNumberingAndLineSeparators(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.md")
	content := " 1. First \r2. Second\r\n３.\u3000第三条\u2028٤. Fourth\u2029\n\v5. Fifth\f6. Sixth\x1c7. Seventh\x1d8. Eighth\x1e9. Ninth\u008510. Tenth\n\x1f11. Eleventh\x1f\n\n# Heading\n- Unnumbered"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := loadRules(path)
	want := []string{"First", "Second", "第三条", "Fourth", "Fifth", "Sixth", "Seventh", "Eighth", "Ninth", "Tenth", "Eleventh", "# Heading", "- Unnumbered"}
	if err != nil || !reflect.DeepEqual(rules, want) {
		t.Fatalf("rules %#v: %v", rules, err)
	}
	if err := saveRules(path, rules); err != nil {
		t.Fatal(err)
	}
	reloaded, err := loadRules(path)
	if err != nil || !reflect.DeepEqual(reloaded, want) {
		t.Fatalf("rules lost on save: %#v %v", reloaded, err)
	}
}

func TestRulesRejectInvalidUTF8WithoutOverwritingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.md")
	content := []byte{'1', '.', ' ', 0xff}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRules(path); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(after, content) {
		t.Fatal("invalid file was overwritten")
	}
}
