package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestParseArgs_PositionalDir(t *testing.T) {
	dir := t.TempDir()
	cfg, err := parseArgs([]string{dir}, io.Discard, "h.json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.root != dir {
		t.Errorf("root = %q, want %q", cfg.root, dir)
	}
	if cfg.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", cfg.timeout)
	}
	if cfg.historyPath != "h.json" {
		t.Errorf("historyPath = %q, want h.json", cfg.historyPath)
	}
}

func TestParseArgs_NoArgsUsesCurrentDir(t *testing.T) {
	cfg, err := parseArgs(nil, io.Discard, "h.json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want, _ := filepath.Abs(".")
	if cfg.root != want {
		t.Errorf("root = %q, want %q", cfg.root, want)
	}
}

func TestParseArgs_FlagsAfterPositional(t *testing.T) {
	dir := t.TempDir()
	cfg, err := parseArgs([]string{dir, "-timeout", "5s"}, io.Discard, "h.json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.root != dir {
		t.Errorf("root = %q, want %q", cfg.root, dir)
	}
	if cfg.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", cfg.timeout)
	}
}

func TestParseArgs_DirFlagAndDifferentPositionalIsError(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	_, err := parseArgs([]string{"-dir", a, b}, io.Discard, "h.json")
	if err == nil || !strings.Contains(err.Error(), "point to different directories") {
		t.Errorf("err = %v, want point to different directories", err)
	}
}

func TestParseArgs_DirFlagAndSamePositionalIsOK(t *testing.T) {
	dir := t.TempDir()
	cfg, err := parseArgs([]string{"-dir", dir, dir + "/"}, io.Discard, "h.json")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cfg.root != dir {
		t.Errorf("root = %q, want %q", cfg.root, dir)
	}
}

func TestParseArgs_TooManyPositionalIsError(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	_, err := parseArgs([]string{a, b}, io.Discard, "h.json")
	if err == nil || err.Error() != "expected at most one directory argument, got 2" {
		t.Errorf("err = %v", err)
	}
}

func TestParseArgs_NonPositiveTimeoutIsError(t *testing.T) {
	_, err := parseArgs([]string{"-timeout", "0"}, io.Discard, "h.json")
	if err == nil || !strings.Contains(err.Error(), "-timeout must be positive") {
		t.Errorf("err = %v, want -timeout must be positive", err)
	}
}

func TestParseArgs_HelpShowsUsage(t *testing.T) {
	var buf bytes.Buffer
	_, err := parseArgs([]string{"-h"}, &buf, "h.json")
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v, want flag.ErrHelp", err)
	}
	for _, want := range []string{
		"browse and run IntelliJ HTTP Client (.http) files",
		"rest-tui [flags] [dir]",
		"rest-tui ./api",
		"Press ? inside the app",
		"history file path",
		"-timeout",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("usage missing %q; got:\n%s", want, buf.String())
		}
	}
}

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		linked string
		info   *debug.BuildInfo
		ok     bool
		want   string
	}{
		{"v1.2.3", nil, false, "v1.2.3"},
		{"dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.2.2"}}, true, "v0.2.2"},
		{"dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, "dev"},
		{"dev", nil, false, "dev"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.linked, c.info, c.ok); got != c.want {
			t.Errorf("resolveVersion(%q, %v, %v) = %q, want %q", c.linked, c.info, c.ok, got, c.want)
		}
	}
}
