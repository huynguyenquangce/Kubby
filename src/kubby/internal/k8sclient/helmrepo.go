package k8sclient

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
)

var helmRepoNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
var helmRepoMu sync.Mutex
var helmRepoPending = map[string]bool{}

func validateHelmRepoName(name string) error {
	if !helmRepoNamePattern.MatchString(name) {
		return fmt.Errorf("repo name must be 1-63 letters, numbers, dots, underscores, or dashes and must start with a letter or number")
	}
	return nil
}

func writeHelmRepoFile(f *repo.File, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o700)
	tmp, err := os.CreateTemp(dir, ".repositories-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := f.WriteFile(tmpPath, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	syncFile, err := os.OpenFile(tmpPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := syncFile.Sync(); err != nil {
		_ = syncFile.Close()
		return err
	}
	if err := syncFile.Close(); err != nil {
		return err
	}
	// Rename inside the same directory is the commit point: a crash cannot leave
	// a partially written repositories.yaml behind.
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true
	// Persist the directory entry where the platform supports syncing a
	// directory. The file is already valid even when that best-effort step is
	// unsupported (notably on Windows).
	if dirHandle, openErr := os.Open(dir); openErr == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}

// downloadHelmRepoIndex keeps Helm's direct cache writes away from the live
// cache. Once both files have been downloaded and the index has parsed, the
// global repository mutex makes the index rename the publication commit point;
// Browse therefore sees either the old complete index or the new one.
func downloadHelmRepoIndex(settings *cli.EnvSettings, entry *repo.Entry) error {
	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		return err
	}
	tempCache, err := os.MkdirTemp(settings.RepositoryCache, ".kubby-repo-index-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempCache)
	r, err := repo.NewChartRepository(entry, getter.All(settings))
	if err != nil {
		return err
	}
	r.CachePath = tempCache
	indexPath, err := r.DownloadIndexFile()
	if err != nil {
		return err
	}
	if _, err := repo.LoadIndexFile(indexPath); err != nil {
		return fmt.Errorf("downloaded index is invalid: %w", err)
	}
	chartsPath := filepath.Join(tempCache, entry.Name+"-charts.txt")

	helmRepoMu.Lock()
	defer helmRepoMu.Unlock()
	if err := publishHelmCacheFile(chartsPath, filepath.Join(settings.RepositoryCache, entry.Name+"-charts.txt")); err != nil {
		return err
	}
	if err := publishHelmCacheFile(indexPath, filepath.Join(settings.RepositoryCache, entry.Name+"-index.yaml")); err != nil {
		return err
	}
	if dir, openErr := os.Open(settings.RepositoryCache); openErr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func publishHelmCacheFile(source, target string) error {
	file, err := os.OpenFile(source, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(source, 0o644); err != nil {
		return err
	}
	return os.Rename(source, target)
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
	helmRepoMu.Lock()
	defer helmRepoMu.Unlock()
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
	helmRepoMu.Lock()
	f, err := loadOrNewRepoFile(settings.RepositoryConfig)
	if err != nil {
		helmRepoMu.Unlock()
		return err
	}
	if f.Get(name) != nil || helmRepoPending[name] {
		helmRepoMu.Unlock()
		return fmt.Errorf("repo %q already exists; remove it first if you intend to replace its URL or credentials", name)
	}
	helmRepoPending[name] = true
	helmRepoMu.Unlock()
	defer func() {
		helmRepoMu.Lock()
		delete(helmRepoPending, name)
		helmRepoMu.Unlock()
	}()

	entry := &repo.Entry{Name: name, URL: url, Username: username, Password: password}
	if err := downloadHelmRepoIndex(settings, entry); err != nil {
		return fmt.Errorf("could not reach repo %q: %w", url, err)
	}

	helmRepoMu.Lock()
	defer helmRepoMu.Unlock()
	f, err = loadOrNewRepoFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}
	if f.Get(name) != nil {
		return fmt.Errorf("repo %q was added while its index was downloading", name)
	}
	f.Update(entry)
	if err := os.MkdirAll(filepath.Dir(settings.RepositoryConfig), 0o700); err != nil {
		return err
	}
	return writeHelmRepoFile(f, settings.RepositoryConfig)
}

// RemoveHelmRepo drops a repo entry and its cached index file.
func RemoveHelmRepo(name string) error {
	helmRepoMu.Lock()
	defer helmRepoMu.Unlock()
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
	helmRepoMu.Lock()
	settings := cli.New()
	f, err := loadOrNewRepoFile(settings.RepositoryConfig)
	helmRepoMu.Unlock()
	if err != nil {
		return err
	}
	entries := append([]*repo.Entry(nil), f.Repositories...)
	var failures []string
	var failuresMu sync.Mutex
	semaphore := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, entry := range entries {
		entry := entry
		wg.Add(1)
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			downloadErr := downloadHelmRepoIndex(settings, entry)
			if downloadErr != nil {
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("%s: %v", entry.Name, downloadErr))
				failuresMu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("some repos failed to update:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

// BrowseHelmRepo lists the charts in a configured repo's cached index (one entry
// per chart, latest version). Run UpdateHelmRepos / AddHelmRepo first to populate
// the cache.
func BrowseHelmRepo(name string) ([]ChartSearchResult, error) {
	helmRepoMu.Lock()
	defer helmRepoMu.Unlock()
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
