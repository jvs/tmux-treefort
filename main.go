package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

func main() {
	// Dispatch named subcommands first.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "show-body":
			runTUIBody(os.Args[2:])
			return
		case "version", "--version":
			fmt.Println("tmux-treefort", version)
			return
		}
	}

	// Default (no args, "show", or flags): open the popup.
	fs := flag.NewFlagSet("tmux-treefort", flag.ExitOnError)
	switchCommand := fs.String("switch-command", "", "command to write to the command file when switching to another tool")
	visitCommand := fs.String("visit-command", "", "execute this command when the user presses enter to confirm selection")
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "show" {
		args = args[1:]
	}
	fs.Parse(args) //nolint:errcheck

	if err := cmdShowPopup(*switchCommand, *visitCommand); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-treefort: %v\n", err)
		os.Exit(1)
	}
}

func runTUIBody(args []string) {
	fs := flag.NewFlagSet("show-body", flag.ExitOnError)
	commandFile := fs.String("command-file", "", "write add-window command here instead of running it")
	returnCommand := fs.String("return-command", "", "append this to the command file after add-window")
	switchCommand := fs.String("switch-command", "", "write this command to the command file when switching to another tool")
	visitCommand := fs.String("visit-command", "", "execute this command when the user presses enter to confirm selection")
	searchMode := fs.Bool("search-mode", false, "start with search focused")
	fs.Parse(args) //nolint:errcheck

	initialSessID, initialWinID, err := getCurrentSessionAndWindow()
	if err != nil {
		fmt.Fprintf(os.Stderr, "tmux-treefort: %v\n", err)
		os.Exit(1)
	}

	m := newModel(initialSessID, initialWinID, *commandFile, *returnCommand, *switchCommand, *visitCommand, *searchMode)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tmux-treefort: %v\n", err)
		os.Exit(1)
	}
}
