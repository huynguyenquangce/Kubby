package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RecentConnection is a remembered kubeconfig source shown on the Welcome
// screen for one-click reconnect. Only file-path connections are stored — pasted
// kubeconfig content is never written to disk (it may contain credentials).
type RecentConnection struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Context string `json:"context"`
}

const maxRecent = 8

func recentFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	kubbyDir := filepath.Join(dir, "kubby")
	if err := os.MkdirAll(kubbyDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(kubbyDir, "recent.json"), nil
}

func loadRecent() []RecentConnection {
	path, err := recentFilePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []RecentConnection
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

func storeRecent(list []RecentConnection) {
	path, err := recentFilePath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

// RecentConnections returns the remembered connections, most-recent first.
func (a *App) RecentConnections() []RecentConnection {
	return loadRecent()
}

// rememberConnection records a successful file-path connection (most-recent
// first, de-duplicated by path+context, capped).
func (a *App) rememberConnection(name, path, context string) {
	if path == "" {
		return // pasted content — never persisted
	}
	list := loadRecent()
	filtered := list[:0]
	for _, r := range list {
		if r.Path == path && r.Context == context {
			continue
		}
		filtered = append(filtered, r)
	}
	updated := append([]RecentConnection{{Name: name, Path: path, Context: context}}, filtered...)
	if len(updated) > maxRecent {
		updated = updated[:maxRecent]
	}
	storeRecent(updated)
}

// ForgetConnection removes a remembered connection.
func (a *App) ForgetConnection(path, context string) {
	list := loadRecent()
	filtered := list[:0]
	for _, r := range list {
		if r.Path == path && r.Context == context {
			continue
		}
		filtered = append(filtered, r)
	}
	storeRecent(filtered)
}
