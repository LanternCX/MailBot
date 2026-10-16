package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"mailbot/internal/mailbot"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		slog.Error("MailBot failed", "error", err)
		os.Exit(1)
	}
}
func run(args []string, input io.Reader, output, errorOutput io.Writer) error {
	flags := flag.NewFlagSet("mailbot", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	path := flags.String("config", "config.json", "Configuration JSON path")
	flags.StringVar(path, "c", "config.json", "Configuration JSON path")
	check := flags.Bool("check", false, "Validate configuration without connecting or creating state")
	showVersion := flags.Bool("version", false, "Print version")
	headless := flags.Bool("headless", false, "Run service without the interactive menu")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if *showVersion {
		fmt.Fprintln(output, "MailBot", version)
		return nil
	}
	abs, err := filepath.Abs(*path)
	if err != nil {
		return err
	}
	if !*headless && !*check {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		return mailbot.RunMenu(ctx, abs, input, output)
	}
	c, err := mailbot.LoadConfig(abs)
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	if *check {
		fmt.Fprintln(output, "Configuration valid")
		return nil
	}
	previousLogger := slog.Default()
	logs, err := mailbot.ConfigureLogging(c.LogLevel, filepath.Join(filepath.Dir(abs), "logs"), errorOutput)
	if err != nil {
		return err
	}
	defer func() { slog.SetDefault(previousLogger); logs.Close() }()
	s, err := mailbot.NewService(c, abs)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return s.Run(ctx)
}
