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
	if err := os.MkdirAll(kubbyDir, 0o700); err != nil {
		return "", err
	}
	_ = os.Chmod(kubbyDir, 0o700)
	return filepath.Join(kubbyDir, "recent.json"), nil
}

func loadRecent(path string) []RecentConnection {
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

func storeRecent(path string, list []RecentConnection) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recent-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func (a *App) recentFile() (string, error) {
	if a.recentPath != nil {
		return a.recentPath()
	}
	return recentFilePath()
}

// RecentConnections returns the remembered connections, most-recent first.
func (a *App) RecentConnections() []RecentConnection {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()
	path, err := a.recentFile()
	if err != nil {
		return nil
	}
	return loadRecent(path)
}

// rememberConnection records a successful file-path connection (most-recent
// first, de-duplicated by path+context, capped).
func (a *App) rememberConnection(name, path, context string) {
	if path == "" {
		return // pasted content — never persisted
	}
	a.recentMu.Lock()
	defer a.recentMu.Unlock()
	recentPath, err := a.recentFile()
	if err != nil {
		return
	}
	list := loadRecent(recentPath)
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
	_ = storeRecent(recentPath, updated)
}

// ForgetConnection removes a remembered connection.
func (a *App) ForgetConnection(path, context string) {
	a.recentMu.Lock()
	defer a.recentMu.Unlock()
	recentPath, err := a.recentFile()
	if err != nil {
		return
	}
	list := loadRecent(recentPath)
	filtered := list[:0]
	for _, r := range list {
		if r.Path == path && r.Context == context {
			continue
		}
		filtered = append(filtered, r)
	}
	_ = storeRecent(recentPath, filtered)
}
