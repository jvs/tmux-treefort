package main

import (
	"fmt"
	"os/exec"
	"strings"
)

type Session struct {
	ID      string
	Name    string
	Windows []Window
}

type Window struct {
	ID        string
	Name      string
	Index     string
	SessionID string
}

func getCurrentSessionAndWindow() (sessID, winID string, err error) {
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_id} #{window_id}").Output()
	if err != nil {
		return "", "", fmt.Errorf("display-message: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected output: %q", string(out))
	}
	return parts[0], parts[1], nil
}

func loadSessions() ([]Session, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_id} #{session_name}").Output()
	if err != nil {
		return nil, err
	}
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		windows, _ := loadWindows(parts[0])
		sessions = append(sessions, Session{ID: parts[0], Name: parts[1], Windows: windows})
	}
	return sessions, nil
}

func loadWindows(sessionID string) ([]Window, error) {
	out, err := exec.Command("tmux", "list-windows", "-t", sessionID, "-F",
		"#{window_id} #{window_index} #{window_name}").Output()
	if err != nil {
		return nil, err
	}
	var windows []Window
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) < 3 {
			continue
		}
		windows = append(windows, Window{
			ID: parts[0], Index: parts[1], Name: parts[2], SessionID: sessionID,
		})
	}
	return windows, nil
}

func tmuxRun(args ...string) error {
	return exec.Command("tmux", args...).Run()
}
