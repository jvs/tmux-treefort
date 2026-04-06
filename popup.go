package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const commandFile = "/tmp/tmux_treefort_command"

func cmdShowPopup(switchCommand, visitCommand string) error {
	exe, err := os.Executable()
	if err != nil {
		exe = "tmux-treefort"
	}

	scriptPath := "/tmp/tmux_treefort_popup.sh"
	if err := writePopupScript(scriptPath, exe, switchCommand, visitCommand); err != nil {
		return fmt.Errorf("writing popup script: %w", err)
	}

	os.Remove(commandFile)

	height, width := calcPopupDimensions()
	tmuxRun("display-popup",
		"-b", "rounded",
		"-h", fmt.Sprintf("%d", height),
		"-w", fmt.Sprintf("%d", width),
		"-T", "#[align=centre fg=white] tmux sessions ",
		"-EE", scriptPath,
	)

	runPendingCommand()
	return nil
}

// writePopupScript writes the shell script that tmux runs inside the popup.
// It calls `tmux-treefort show-body` with the appropriate flags.
func writePopupScript(path, exe, switchCommand, visitCommand string) error {
	cmd := fmt.Sprintf("exec %s show-body --command-file %s --return-command %q",
		exe, commandFile, exe+" show")
	if switchCommand != "" {
		cmd += fmt.Sprintf(" --switch-command %q", switchCommand)
	}
	if visitCommand != "" {
		cmd += fmt.Sprintf(" --visit-command %q", visitCommand)
	}
	body := "#!/bin/sh\n" + cmd + "\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		return err
	}
	return os.Chmod(path, 0755)
}

// calcPopupDimensions computes the popup height and width from the current
// tmux state.
func calcPopupDimensions() (height, width int) {
	// Collect session names, excluding internal __ sessions.
	sessOut, _ := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	var sessionNames []string
	for _, name := range strings.Split(strings.TrimSpace(string(sessOut)), "\n") {
		if name != "" && !strings.HasPrefix(name, "__") {
			sessionNames = append(sessionNames, name)
		}
	}

	// Collect window names in non-__ sessions.
	winOut, _ := exec.Command("tmux", "list-windows", "-a", "-F", "#{session_name} #{window_name}").Output()
	var windowCount int
	longestWinName := 0
	for _, line := range strings.Split(strings.TrimSpace(string(winOut)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 || strings.HasPrefix(parts[0], "__") {
			continue
		}
		windowCount++
		if l := len(parts[1]); l > longestWinName {
			longestWinName = l
		}
	}

	longestSessName := 0
	for _, name := range sessionNames {
		if l := len(name); l > longestSessName {
			longestSessName = l
		}
	}

	// height = total item count + 4 (top/bottom border + blank line + search bar)
	height = len(sessionNames) + windowCount + 4

	// width = max(longest name + 3, 24) + 6
	longestName := longestSessName
	if longestWinName > longestName {
		longestName = longestWinName
	}
	longestName += 3
	if longestName < 24 {
		longestName = 24
	}
	width = longestName + 6

	return height, width
}

// runPendingCommand reads commandFile and executes its contents via sh.
// The file is removed before execution so the command can write a new one.
func runPendingCommand() {
	data, err := os.ReadFile(commandFile)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return
	}
	os.Remove(commandFile)
	exec.Command("sh", "-c", string(data)).Run()
}
