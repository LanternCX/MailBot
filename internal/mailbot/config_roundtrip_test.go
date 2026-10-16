package mailbot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelativeConfigSavePreservesStateDatabaseLocation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := SaveConfig(path, DefaultConfig()); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(relative)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "mailbot.sqlite3")
	if c.StatePath != want {
		t.Fatalf("unexpected state path %q, want %q", c.StatePath, want)
	}
	for range 3 {
		c.PollInterval++
		if err := SaveConfig(relative, c); err != nil {
			t.Fatal(err)
		}
		c, err = LoadConfig(relative)
		if err != nil {
			t.Fatal(err)
		}
		if c.StatePath != want {
			t.Fatalf("state moved after configuration save: %q", c.StatePath)
		}
	}
}
