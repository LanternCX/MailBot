package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckReadsLegacyConfigurationWithoutCreatingStateOrLogs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.json")
	config := `{"poll_interval":60,"accounts":[],"notifiers":[],"ai":{"enabled":false}}`
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	var output, errors bytes.Buffer
	if err := run([]string{"--check", "-c", path}, strings.NewReader(""), &output, &errors); err != nil {
		t.Fatal(err)
	}
	if output.String() != "Configuration valid\n" {
		t.Fatal("check command did not report success")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("--check created runtime artifacts: %v", entries)
	}
}

func TestDefaultMenuWorksWithoutConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	var output bytes.Buffer
	if err := run([]string{"--config", path}, strings.NewReader("7\n"), &output, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Main Menu") || !strings.Contains(output.String(), "Bye.") {
		t.Fatal("default launch is not interactive")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("exiting a new menu wrote a configuration")
	}
}

func TestHeadlessRejectsMissingOrUnusableConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	var output bytes.Buffer
	if err := run([]string{"--headless", "-c", path}, strings.NewReader("7\n"), &output, &output); err == nil {
		t.Fatal("headless accepted a missing configuration")
	}
	os.WriteFile(path, []byte(`{"accounts":[],"notifiers":[]}`), 0600)
	if err := run([]string{"--headless", "-c", path}, strings.NewReader("7\n"), &output, &output); err == nil || !strings.Contains(err.Error(), "Telegram") {
		t.Fatalf("headless should reject missing target: %v", err)
	}
	if strings.Contains(output.String(), "Main Menu") {
		t.Fatal("headless fell back to interactive mode")
	}
}

func TestHelpAndVersionDoNotRequireConfiguration(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		var output bytes.Buffer
		if err := run([]string{arg, "-c", filepath.Join(t.TempDir(), "missing.json")}, strings.NewReader(""), &output, &output); err != nil {
			t.Fatal(err)
		}
		if output.Len() == 0 {
			t.Fatalf("empty %s output", arg)
		}
	}
}

func TestInvalidConfigAndUnknownOptionsAreErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"poll_interval":1}`), 0600)
	for _, args := range [][]string{{"--headless", "-c", path}, {"--check", "-c", path}, {"--unknown"}, {"unexpected"}} {
		var output bytes.Buffer
		if err := run(args, strings.NewReader(""), &output, &output); err == nil {
			t.Fatalf("accepted invalid invocation %v", args)
		}
	}
}
