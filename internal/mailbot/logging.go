package mailbot

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type rotatingLog struct {
	mu   sync.Mutex
	file *os.File
	path string
	size int64
}

func (w *rotatingLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > 5*1024*1024 {
		if err := w.file.Close(); err != nil {
			return 0, err
		}
		os.Remove(w.path + ".3")
		for i := 2; i >= 1; i-- {
			if err := os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1)); err != nil && !os.IsNotExist(err) {
				return 0, err
			}
		}
		if err := os.Rename(w.path, w.path+".1"); err != nil {
			return 0, err
		}
		var err error
		w.file, err = os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		w.size = 0
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}
func (w *rotatingLog) Close() error { w.mu.Lock(); defer w.mu.Unlock(); return w.file.Close() }

type logPair struct{ console, file slog.Handler }

func (h logPair) Enabled(ctx context.Context, level slog.Level) bool {
	return h.console.Enabled(ctx, level) || h.file.Enabled(ctx, level)
}
func (h logPair) Handle(ctx context.Context, r slog.Record) error {
	var first error
	if h.console.Enabled(ctx, r.Level) {
		first = h.console.Handle(ctx, r.Clone())
	}
	if h.file.Enabled(ctx, r.Level) {
		if err := h.file.Handle(ctx, r); first == nil {
			first = err
		}
	}
	return first
}
func (h logPair) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logPair{h.console.WithAttrs(attrs), h.file.WithAttrs(attrs)}
}
func (h logPair) WithGroup(name string) slog.Handler {
	return logPair{h.console.WithGroup(name), h.file.WithGroup(name)}
}

// File logs always include DEBUG, while console logs follow the configured level.
func ConfigureLogging(level, dir string, console io.Writer) (io.Closer, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "mailbot.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	w := &rotatingLog{file: f, path: path, size: info.Size()}
	l := slog.LevelInfo
	switch strings.ToUpper(level) {
	case "DEBUG":
		l = slog.LevelDebug
	case "WARNING":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	case "CRITICAL":
		l = slog.Level(12)
	}
	slog.SetDefault(slog.New(logPair{slog.NewTextHandler(console, &slog.HandlerOptions{Level: l}), slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})}))
	return w, nil
}
