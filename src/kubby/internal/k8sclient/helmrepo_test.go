package k8sclient

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/repo"
)

const testHelmIndex = "apiVersion: v1\nentries: {}\n"

func TestValidateHelmRepoNameRejectsCachePathEscape(t *testing.T) {
	for _, name := range []string{"../outside", "team/repo", `team\\repo`, "", ".hidden"} {
		if err := validateHelmRepoName(name); err == nil {
			t.Errorf("expected repo name %q to be rejected", name)
		}
	}
	for _, name := range []string{"stable", "team-repo_2", "charts.example.com"} {
		if err := validateHelmRepoName(name); err != nil {
			t.Errorf("expected repo name %q to be accepted: %v", name, err)
		}
	}
}

func TestWriteHelmRepoFileIsPrivateAndLoadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helm", "repositories.yaml")
	f := repo.NewFile()
	f.Update(&repo.Entry{Name: "private", URL: "https://charts.example.test", Username: "user", Password: "sentinel"})
	if err := writeHelmRepoFile(f, path); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.LoadFile(path)
	if err != nil || loaded.Get("private") == nil {
		t.Fatalf("atomic repo file was not loadable: loaded=%#v err=%v", loaded, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("repo file mode = %v; want 0600", info.Mode().Perm())
	}
}

func TestConfigureChartSourceCarriesConfiguredCredentials(t *testing.T) {
	config := filepath.Join(t.TempDir(), "repositories.yaml")
	t.Setenv("HELM_REPOSITORY_CONFIG", config)
	f := repo.NewFile()
	f.Update(&repo.Entry{
		Name: "private", URL: "https://private.example.test/charts",
		Username: "reader", Password: "password-sentinel", PassCredentialsAll: true,
	})
	if err := writeHelmRepoFile(f, config); err != nil {
		t.Fatal(err)
	}
	options := action.ChartPathOptions{}
	if err := configureChartSource(&options, "https://stale.invalid", "1.2.3", "private"); err != nil {
		t.Fatal(err)
	}
	if options.RepoURL != "https://private.example.test/charts" || options.Username != "reader" || options.Password != "password-sentinel" || !options.PassCredentialsAll {
		t.Fatalf("configured source lost repository auth: %#v", options)
	}
}

func TestAddHelmRepoDoesNotHoldConfigLockDuringDownload(t *testing.T) {
	temp := t.TempDir()
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(temp, "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(temp, "cache"))
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce, releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		startedOnce.Do(func() { close(started) })
		<-release
		_, _ = w.Write([]byte(testHelmIndex))
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })

	addDone := make(chan error, 1)
	go func() { addDone <- AddHelmRepo("slow", server.URL, "", "") }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("repo download did not start")
	}
	listDone := make(chan error, 1)
	go func() {
		_, err := ListHelmRepos()
		listDone <- err
	}()
	select {
	case err := <-listDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		releaseOnce.Do(func() { close(release) })
		t.Fatal("ListHelmRepos blocked behind an in-flight index download")
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-addDone; err != nil {
		t.Fatal(err)
	}
}

func TestUpdateHelmReposDownloadsIndexesConcurrently(t *testing.T) {
	temp := t.TempDir()
	config := filepath.Join(temp, "repositories.yaml")
	t.Setenv("HELM_REPOSITORY_CONFIG", config)
	t.Setenv("HELM_REPOSITORY_CACHE", filepath.Join(temp, "cache"))
	started := make(chan string, 4)
	release := make(chan struct{})
	var releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		started <- request.URL.Path
		<-release
		_, _ = w.Write([]byte(testHelmIndex))
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	f := repo.NewFile()
	for i := 0; i < 4; i++ {
		name := string(rune('a' + i))
		f.Update(&repo.Entry{Name: name, URL: server.URL + "/" + name})
	}
	if err := writeHelmRepoFile(f, config); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- UpdateHelmRepos() }()
	seen := map[string]bool{}
	for len(seen) < 4 {
		select {
		case path := <-started:
			seen[path] = true
		case <-time.After(2 * time.Second):
			releaseOnce.Do(func() { close(release) })
			t.Fatalf("only %d/4 index downloads started concurrently: %v", len(seen), seen)
		}
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
