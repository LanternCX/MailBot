// Command package builds a standalone binary and a portable release archive.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mailbot/internal/mailbot"
)

type platform struct{ os, arch string }

var platforms = map[string]platform{"linux-x64": {"linux", "amd64"}, "linux-arm64": {"linux", "arm64"}, "macos-arm64": {"darwin", "arm64"}, "windows-x64": {"windows", "amd64"}}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	variant := flag.String("variant", "linux-x64", "linux-x64, linux-arm64, macos-arm64 or windows-x64")
	name := flag.String("name", "", "Executable name (default: mailbot-PLATFORM)")
	tag := flag.String("tag", "dev", "Version embedded in binary")
	entry := flag.String("entry", "./cmd/mailbot", "Go main package")
	clean := flag.Bool("clean", false, "Replace this platform's build directory")
	flag.Parse()
	p, ok := platforms[*variant]
	if !ok {
		return fmt.Errorf("unsupported variant %q", *variant)
	}
	if strings.ContainsAny(*tag, "\r\n\t \"'`/\\") {
		return fmt.Errorf("tag must not contain whitespace or quotes")
	}
	bin := *name
	if bin == "" {
		bin = "mailbot-" + *variant
	}
	if filepath.Base(bin) != bin || bin == "." || bin == ".." {
		return fmt.Errorf("name must be a filename")
	}
	if p.os == "windows" && !strings.HasSuffix(bin, ".exe") {
		bin += ".exe"
	}
	dir := filepath.Join("dist", *variant)
	if *clean {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, bin)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w -X main.version="+*tag, "-o", path, *entry)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+p.os, "GOARCH="+p.arch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	archive := filepath.Join("dist", "mailbot-"+*tag+"-"+*variant+".zip")
	if err := bundle(archive, path, bin); err != nil {
		return err
	}
	fmt.Println(archive)
	return nil
}

func bundle(archive, binary, name string) error {
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	finish := func(err error) error {
		ze := z.Close()
		fe := f.Close()
		if err != nil {
			return err
		}
		if ze != nil {
			return ze
		}
		return fe
	}
	add := func(name string, mode os.FileMode, r io.Reader) error {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(mode)
		w, err := z.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, r)
		return err
	}
	b, err := os.Open(binary)
	if err != nil {
		return finish(err)
	}
	err = add(name, 0755, b)
	b.Close()
	if err != nil {
		return finish(err)
	}
	c, err := json.MarshalIndent(mailbot.DefaultConfig(), "", "  ")
	if err != nil {
		return finish(err)
	}
	if err = add("config.json", 0600, bytes.NewReader(append(c, '\n'))); err != nil {
		return finish(err)
	}
	for _, entry := range []struct{ source, name string }{
		{"config.example.json", "config.example.json"}, {"deploy/mailbot.service", "mailbot.service"}, {"LICENSE", "LICENSE"},
		{"README.md", "README.md"}, {"README_ZH.md", "README_ZH.md"},
		{"docs/zh/go-service.md", "docs/zh/go-service.md"}, {"docs/zh/configuration.md", "docs/zh/configuration.md"}, {"docs/zh/setup_guide.md", "docs/zh/setup_guide.md"},
		{"docs/en/configuration.md", "docs/en/configuration.md"}, {"docs/en/setup_guide.md", "docs/en/setup_guide.md"}, {"docs/dev/development_zh.md", "docs/dev/development_zh.md"},
	} {
		r, err := os.Open(entry.source)
		if err != nil {
			return finish(err)
		}
		err = add(entry.name, 0644, r)
		r.Close()
		if err != nil {
			return finish(err)
		}
	}
	return finish(nil)
}
