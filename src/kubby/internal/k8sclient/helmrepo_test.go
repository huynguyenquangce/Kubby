package k8sclient

import "testing"

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
