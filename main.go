// Command rest-tui is a full-screen terminal app for browsing and running
// IntelliJ HTTP Client (.http) scratch files without leaving the terminal.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Ahngbeom/rest-tui/internal/history"
	"github.com/Ahngbeom/rest-tui/internal/tui"
)

func defaultHistoryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rest-tui", "history.json"), nil
}

func main() {
	defaultHistory, err := defaultHistoryPath()
	if err != nil {
		defaultHistory = "history.json"
	}

	dir := flag.String("dir", ".", "directory to search for .http files")
	historyPath := flag.String("config", defaultHistory, "path to the history file")
	flag.Parse()

	root, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rest-tui:", err)
		os.Exit(1)
	}

	store := history.NewStore(*historyPath)
	app := tui.NewApp(root, store)

	if _, err := tea.NewProgram(app, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "rest-tui:", err)
		os.Exit(1)
	}
}
