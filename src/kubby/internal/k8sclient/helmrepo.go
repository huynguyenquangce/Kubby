package k8sclient

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

var helmRepoNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func validateHelmRepoName(name string) error {
	if !helmRepoNamePattern.MatchString(name) {
		return fmt.Errorf("repo name must be 1-63 letters, numbers, dots, underscores, or dashes and must start with a letter or number")
	}
	return nil
}

func writeHelmRepoFile(f *repo.File, path string) error {
	if err := f.WriteFile(path, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// HelmRepo is one configured chart repository.
type HelmRepo struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	Authenticated bool   `json:"authenticated"`
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
		out = append(out, HelmRepo{Name: e.Name, URL: e.URL, Authenticated: e.Username != "" || e.Password != "" || e.CertFile != ""})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AddHelmRepo adds a repo entry and downloads its index to validate the URL.
// Reusing a name is an explicit conflict: silently replacing it would also
// mutate the user's Helm CLI configuration and could discard credentials.
func AddHelmRepo(name, url, username, password string) error {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if name == "" || url == "" {
		return fmt.Errorf("repo name and URL are required")
	}
	if err := validateHelmRepoName(name); err != nil {
		return err
	}
	settings := cli.New()

	f, err := loadOrNewRepoFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}
	if f.Get(name) != nil {
		return fmt.Errorf("repo %q already exists; remove it first if you intend to replace its URL or credentials", name)
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
	return writeHelmRepoFile(f, settings.RepositoryConfig)
}

// RemoveHelmRepo drops a repo entry and its cached index file.
func RemoveHelmRepo(name string) error {
	if err := validateHelmRepoName(name); err != nil {
		return err
	}
	settings := cli.New()
	f, err := repo.LoadFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}
	if !f.Remove(name) {
		return fmt.Errorf("repo %q not found", name)
	}
	if err := writeHelmRepoFile(f, settings.RepositoryConfig); err != nil {
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
	if err := validateHelmRepoName(name); err != nil {
		return nil, err
	}
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
		availableVersions := make([]string, 0, len(versions))
		for _, available := range versions {
			availableVersions = append(availableVersions, available.Version)
		}
		out = append(out, ChartSearchResult{
			Name:        chartName,
			NormName:    chartName,
			Repo:        name,
			RepoURL:     entry.URL,
			SourceID:    name,
			Version:     v.Version,
			AppVersion:  v.AppVersion,
			Description: v.Description,
			Versions:    availableVersions,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
