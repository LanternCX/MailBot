package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleIncludesSafeDefaultConfigAndExecutable(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	dir := t.TempDir()
	binary := filepath.Join(dir, "binary")
	if err := os.WriteFile(binary, []byte("binary bytes"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "bundle.zip")
	if err := bundle(path, binary, "mailbot-linux-x64"); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	for _, name := range []string{"mailbot-linux-x64", "config.json", "config.example.json", "mailbot.service", "docs/zh/go-service.md", "README.md", "LICENSE"} {
		if files[name] == nil {
			t.Fatalf("missing artifact %s", name)
		}
	}
	if files["mailbot-linux-x64"].Mode().Perm() != 0755 {
		t.Fatal("binary lost executable permission")
	}
	r, _ := files["config.json"].Open()
	data, _ := io.ReadAll(r)
	r.Close()
	var c struct {
		Accounts  []any `json:"accounts"`
		Notifiers []any `json:"notifiers"`
	}
	if err = json.Unmarshal(data, &c); err != nil || len(c.Accounts) != 0 || len(c.Notifiers) != 0 {
		t.Fatal("bundle should open unconfigured wizard, not ship configured accounts")
	}
}
