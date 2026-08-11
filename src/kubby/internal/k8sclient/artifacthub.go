package k8sclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ChartSearchResult is one chart returned from an Artifact Hub search.
type ChartSearchResult struct {
	Name        string `json:"name"`
	NormName    string `json:"normalizedName"`
	Repo        string `json:"repo"`
	RepoURL     string `json:"repoURL"`
	Version     string `json:"version"`
	AppVersion  string `json:"appVersion"`
	Description string `json:"description"`
	Stars       int    `json:"stars"`
}

// SearchCharts queries Artifact Hub for Helm charts matching the query.
// Requires outbound internet access to artifacthub.io.
func SearchCharts(ctx context.Context, query string) ([]ChartSearchResult, error) {
	endpoint := "https://artifacthub.io/api/v1/packages/search?kind=0&limit=25&sort=relevance&ts_query_web=" +
		url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Artifact Hub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Artifact Hub returned status %d", resp.StatusCode)
	}

	var payload struct {
		Packages []struct {
			Name        string `json:"name"`
			NormName    string `json:"normalized_name"`
			Version     string `json:"version"`
			AppVersion  string `json:"app_version"`
			Description string `json:"description"`
			Stars       int    `json:"stars"`
			Repository  struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"repository"`
		} `json:"packages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	out := make([]ChartSearchResult, 0, len(payload.Packages))
	for _, p := range payload.Packages {
		out = append(out, ChartSearchResult{
			Name: p.Name, NormName: p.NormName, Repo: p.Repository.Name, RepoURL: p.Repository.URL,
			Version: p.Version, AppVersion: p.AppVersion, Description: p.Description, Stars: p.Stars,
		})
	}
	return out, nil
}

// ChartLink is a named external link for a chart (home, source, docs…).
type ChartLink struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ChartDetail is the rich detail of one chart, used to inform an install:
// default values, README, the available versions, and reference links.
type ChartDetail struct {
	Name          string      `json:"name"`
	Repo          string      `json:"repo"`
	RepoURL       string      `json:"repoURL"`
	Version       string      `json:"version"`
	AppVersion    string      `json:"appVersion"`
	Description   string      `json:"description"`
	HomeURL       string      `json:"homeURL"`
	Readme        string      `json:"readme"`
	DefaultValues string      `json:"defaultValues"`
	Maintainers   []string    `json:"maintainers"`
	Keywords      []string    `json:"keywords"`
	Links         []ChartLink `json:"links"`
	Versions      []string    `json:"versions"`
}

// ChartDetails fetches Artifact Hub's package detail for one chart (repo +
// normalized name). Requires internet access to artifacthub.io.
func ChartDetails(ctx context.Context, repoName, chartName string) (*ChartDetail, error) {
	endpoint := fmt.Sprintf("https://artifacthub.io/api/v1/packages/helm/%s/%s",
		url.PathEscape(repoName), url.PathEscape(chartName))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Artifact Hub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Artifact Hub returned status %d", resp.StatusCode)
	}

	var p struct {
		Name          string   `json:"name"`
		DisplayName   string   `json:"display_name"`
		Description   string   `json:"description"`
		Version       string   `json:"version"`
		AppVersion    string   `json:"app_version"`
		HomeURL       string   `json:"home_url"`
		Readme        string   `json:"readme"`
		DefaultValues string   `json:"default_values"`
		Keywords      []string `json:"keywords"`
		Repository    struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"repository"`
		Maintainers []struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"maintainers"`
		Links []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"links"`
		AvailableVersions []struct {
			Version string `json:"version"`
		} `json:"available_versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}

	d := &ChartDetail{
		Name: p.Name, Repo: p.Repository.Name, RepoURL: p.Repository.URL,
		Version: p.Version, AppVersion: p.AppVersion, Description: p.Description,
		HomeURL: p.HomeURL, Readme: p.Readme, DefaultValues: p.DefaultValues, Keywords: p.Keywords,
	}
	for _, m := range p.Maintainers {
		if m.Name != "" {
			d.Maintainers = append(d.Maintainers, m.Name)
		}
	}
	for _, l := range p.Links {
		d.Links = append(d.Links, ChartLink{Name: l.Name, URL: l.URL})
	}
	for _, v := range p.AvailableVersions {
		d.Versions = append(d.Versions, v.Version)
	}
	return d, nil
}
