// Command rest-tui is a full-screen terminal app for browsing and running
// IntelliJ HTTP Client (.http) scratch files without leaving the terminal.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/dirhistory"
	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func defaultHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rest-tui", "history.json"), nil
}

func defaultDirHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rest-tui", "dirs.json"), nil
}

const usageHeader = `rest-tui - browse and run IntelliJ HTTP Client (.http) files in the terminal

Usage:
  rest-tui [flags] [dir]

Examples:
  rest-tui                 scan the current directory
  rest-tui ./api           scan ./api
  rest-tui -timeout 2m .   allow slow requests

Press ? inside the app to see all keys.

Flags:
`

type cliConfig struct {
	root, historyPath string
	timeout           time.Duration
	showVersion       bool
}

func parseArgs(args []string, stderr io.Writer, defaultHistory string) (cliConfig, error) {
	fs := flag.NewFlagSet("rest-tui", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "directory to search for .http files")
	historyPath := fs.String("config", defaultHistory, "history file path")
	timeout := fs.Duration("timeout", 30*time.Second, "per-request timeout, e.g. 10s or 2m")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usageHeader)
		fs.PrintDefaults()
	}

	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return cliConfig{}, err
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}

	if len(positional) > 1 {
		return cliConfig{}, fmt.Errorf("expected at most one directory argument, got %d", len(positional))
	}
	if *timeout <= 0 {
		return cliConfig{}, fmt.Errorf("-timeout must be positive, got %s", *timeout)
	}

	root, err := filepath.Abs(*dir)
	if err != nil {
		return cliConfig{}, err
	}
	if len(positional) == 1 {
		argRoot, err := filepath.Abs(positional[0])
		if err != nil {
			return cliConfig{}, err
		}
		dirSet := false
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "dir" {
				dirSet = true
			}
		})
		if dirSet && argRoot != root {
			return cliConfig{}, fmt.Errorf("-dir %s and argument %s point to different directories", *dir, positional[0])
		}
		root = argRoot
	}

	return cliConfig{root: root, historyPath: *historyPath, timeout: *timeout, showVersion: *showVersion}, nil
}

func resolveVersion(linked string, info *debug.BuildInfo, ok bool) string {
	if linked != "dev" {
		return linked
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func main() {
	defaultHistory, err := defaultHistoryPath()
	if err != nil {
		defaultHistory = "history.json"
	}
	defaultDirHistory, err := defaultDirHistoryPath()
	if err != nil {
		defaultDirHistory = "dirs.json"
	}

	cfg, err := parseArgs(os.Args[1:], os.Stderr, defaultHistory)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rest-tui:", err)
		os.Exit(2)
	}

	if cfg.showVersion {
		info, ok := debug.ReadBuildInfo()
		fmt.Println("rest-tui", resolveVersion(version, info, ok))
		return
	}

	store := history.NewStore(cfg.historyPath)
	dirStore := dirhistory.NewStore(defaultDirHistory)
	app := tui.NewApp(cfg.root, store, dirStore, tui.Options{Timeout: cfg.timeout})

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "rest-tui:", err)
		os.Exit(1)
	}
}
