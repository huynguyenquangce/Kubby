package k8sclient

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

// HelmRepo is one configured chart repository.
type HelmRepo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// loadOrNewRepoFile loads the Helm repositories.yaml, returning a fresh empty
// file if it doesn't exist yet (checked via os.Stat because repo.LoadFile wraps
// the OS "not found" error in a way os.IsNotExist can't see on Windows).
func loadOrNewRepoFile(path string) (*repo.File, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return repo.NewFile(), nil
	}
	return repo.LoadFile(path)
}

// ListHelmRepos returns the repos configured in the user's Helm repositories.yaml.
// An absent file means no repos configured yet (not an error).
func ListHelmRepos() ([]HelmRepo, error) {
	settings := cli.New()
	f, err := loadOrNewRepoFile(settings.RepositoryConfig)
	if err != nil {
		return nil, err
	}
	out := make([]HelmRepo, 0, len(f.Repositories))
	for _, e := range f.Repositories {
		out = append(out, HelmRepo{Name: e.Name, URL: e.URL})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AddHelmRepo adds (or updates) a repo entry and downloads its index to validate
// the URL is reachable. username/password are optional (private repos).
func AddHelmRepo(name, url, username, password string) error {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if name == "" || url == "" {
		return fmt.Errorf("repo name and URL are required")
	}
	settings := cli.New()

	f, err := loadOrNewRepoFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}

	entry := &repo.Entry{Name: name, URL: url, Username: username, Password: password}
	r, err := repo.NewChartRepository(entry, getter.All(settings))
	if err != nil {
		return err
	}
	r.CachePath = settings.RepositoryCache
	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return err
	}
	if _, err := r.DownloadIndexFile(); err != nil {
		return fmt.Errorf("could not reach repo %q: %w", url, err)
	}

	f.Update(entry)
	if err := os.MkdirAll(filepath.Dir(settings.RepositoryConfig), 0o755); err != nil {
		return err
	}
	return f.WriteFile(settings.RepositoryConfig, 0o644)
}

// RemoveHelmRepo drops a repo entry and its cached index file.
func RemoveHelmRepo(name string) error {
	settings := cli.New()
	f, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}
	if !f.Remove(name) {
		return fmt.Errorf("repo %q not found", name)
	}
	if err := f.WriteFile(settings.RepositoryConfig, 0o644); err != nil {
		return err
	}
	// Best-effort cache cleanup.
	_ = os.Remove(filepath.Join(settings.RepositoryCache, name+"-index.yaml"))
	_ = os.Remove(filepath.Join(settings.RepositoryCache, name+"-charts.txt"))
	return nil
}

// UpdateHelmRepos re-downloads the index for every configured repo.
func UpdateHelmRepos() error {
	settings := cli.New()
	f, err := loadOrNewRepoFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}
	var failures []string
	for _, e := range f.Repositories {
		r, err := repo.NewChartRepository(e, getter.All(settings))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", e.Name, err))
			continue
		}
		r.CachePath = settings.RepositoryCache
		if _, err := r.DownloadIndexFile(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", e.Name, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("some repos failed to update:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

// BrowseHelmRepo lists the charts in a configured repo's cached index (one entry
// per chart, latest version). Run UpdateHelmRepos / AddHelmRepo first to populate
// the cache.
func BrowseHelmRepo(name string) ([]ChartSearchResult, error) {
	settings := cli.New()
	f, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		return nil, err
	}
	entry := f.Get(name)
	if entry == nil {
		return nil, fmt.Errorf("repo %q not found", name)
	}
	idxPath := filepath.Join(settings.RepositoryCache, name+"-index.yaml")
	idx, err := repo.LoadIndexFile(idxPath)
	if err != nil {
		return nil, fmt.Errorf("no cached index for %q — try Update first: %w", name, err)
	}
	idx.SortEntries()

	out := make([]ChartSearchResult, 0, len(idx.Entries))
	for chartName, versions := range idx.Entries {
		if len(versions) == 0 {
			continue
		}
		v := versions[0] // latest after SortEntries
		out = append(out, ChartSearchResult{
			Name:        chartName,
			NormName:    chartName,
			Repo:        name,
			RepoURL:     entry.URL,
			Version:     v.Version,
			AppVersion:  v.AppVersion,
			Description: v.Description,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
