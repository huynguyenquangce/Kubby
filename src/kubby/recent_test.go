package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRecentConnectionsSerializeConcurrentUpdatesAndStayPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	a := NewApp()
	a.recentPath = func() (string, error) { return path, nil }
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a.rememberConnection(fmt.Sprintf("cluster-%d", i), fmt.Sprintf("/configs/%d", i), "default")
		}(i)
	}
	wg.Wait()
	got := a.RecentConnections()
	if len(got) != maxRecent {
		t.Fatalf("recent connections = %d, want capped %d", len(got), maxRecent)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("recent file mode = %o, want 600", info.Mode().Perm())
	}
}
