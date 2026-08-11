package k8sclient

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"os"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

// restClientGetter adapts our *rest.Config to Helm's RESTClientGetter so the
// Helm SDK talks to the same cluster Kubby is connected to (no kubeconfig file).
type restClientGetter struct {
	cfg       *rest.Config
	namespace string
}

func (g *restClientGetter) ToRESTConfig() (*rest.Config, error) { return g.cfg, nil }

func (g *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(g.cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (g *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

func (g *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	overrides := &clientcmd.ConfigOverrides{Context: clientcmdapi.Context{Namespace: g.namespace}}
	return clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), overrides)
}

func newHelmConfig(c *Cluster, namespace string) (*action.Configuration, error) {
	if namespace == "" {
		namespace = "default"
	}
	getter := &restClientGetter{cfg: c.Rest, namespace: namespace}
	cfg := new(action.Configuration)
	if err := cfg.Init(getter, namespace, "secret", func(string, ...interface{}) {}); err != nil {
		return nil, err
	}
	return cfg, nil
}

// HelmReleaseDetail is the full detail of one release (values/manifest/notes).
type HelmReleaseDetail struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Revision   int    `json:"revision"`
	Status     string `json:"status"`
	Chart      string `json:"chart"`
	AppVersion string `json:"appVersion"`
	Notes      string `json:"notes"`
	Values     string `json:"values"`
	Manifest   string `json:"manifest"`
}

func HelmGet(c *Cluster, namespace, name string) (*HelmReleaseDetail, error) {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return nil, err
	}
	rel, err := action.NewGet(cfg).Run(name)
	if err != nil {
		return nil, err
	}
	valuesYAML, _ := yaml.Marshal(rel.Config)
	chart := ""
	if rel.Chart != nil && rel.Chart.Metadata != nil {
		chart = rel.Chart.Metadata.Name + "-" + rel.Chart.Metadata.Version
	}
	appVer := ""
	if rel.Chart != nil && rel.Chart.Metadata != nil {
		appVer = rel.Chart.Metadata.AppVersion
	}
	return &HelmReleaseDetail{
		Name: rel.Name, Namespace: rel.Namespace, Revision: rel.Version,
		Status: rel.Info.Status.String(), Chart: chart, AppVersion: appVer,
		Notes: rel.Info.Notes, Values: string(valuesYAML), Manifest: rel.Manifest,
	}, nil
}

// HelmRevision is one entry in a release's history.
type HelmRevision struct {
	Revision    int    `json:"revision"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	Updated     string `json:"updated"`
	Description string `json:"description"`
}

func HelmHistory(c *Cluster, namespace, name string) ([]HelmRevision, error) {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return nil, err
	}
	rels, err := action.NewHistory(cfg).Run(name)
	if err != nil {
		return nil, err
	}
	out := make([]HelmRevision, 0, len(rels))
	for _, rel := range rels {
		chart := ""
		if rel.Chart != nil && rel.Chart.Metadata != nil {
			chart = rel.Chart.Metadata.Name + "-" + rel.Chart.Metadata.Version
		}
		out = append(out, HelmRevision{
			Revision: rel.Version, Status: rel.Info.Status.String(), Chart: chart,
			Updated: age(metav1.Time{Time: rel.Info.LastDeployed.Time}), Description: rel.Info.Description,
		})
	}
	// Newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func HelmRollback(c *Cluster, namespace, name string, revision int) error {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return err
	}
	rb := action.NewRollback(cfg)
	rb.Version = revision
	return rb.Run(name)
}

func HelmUninstall(c *Cluster, namespace, name string) error {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return err
	}
	_, err = action.NewUninstall(cfg).Run(name)
	return err
}

// HelmUpgradeValues re-runs a release with new values, reusing its current chart.
func HelmUpgradeValues(c *Cluster, namespace, name, valuesYAML string) error {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return err
	}
	rel, err := action.NewGet(cfg).Run(name)
	if err != nil {
		return err
	}
	vals := map[string]interface{}{}
	if valuesYAML != "" {
		if err := yaml.Unmarshal([]byte(valuesYAML), &vals); err != nil {
			return err
		}
	}
	up := action.NewUpgrade(cfg)
	up.Namespace = namespace
	_, err = up.Run(name, rel.Chart, vals)
	return err
}

// parseValues turns an optional YAML string into a values map.
func parseValues(valuesYAML string) (map[string]interface{}, error) {
	vals := map[string]interface{}{}
	if strings.TrimSpace(valuesYAML) != "" {
		if err := yaml.Unmarshal([]byte(valuesYAML), &vals); err != nil {
			return nil, fmt.Errorf("invalid values YAML: %w", err)
		}
	}
	return vals, nil
}

// locateAndLoadChart resolves a chart from a repo URL, fingerprints the exact
// archive bytes, and loads it into memory. The digest binds a preview to the
// artifact that a later install is allowed to write.
func locateAndLoadChart(inst *action.Install, repoURL, chartName, version string) (*chart.Chart, string, error) {
	configureChartSource(&inst.ChartPathOptions, repoURL, version)
	chartPath, err := inst.ChartPathOptions.LocateChart(chartName, cli.New())
	if err != nil {
		return nil, "", err
	}
	archive, err := os.ReadFile(chartPath)
	if err != nil {
		return nil, "", fmt.Errorf("read resolved chart archive: %w", err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(archive))
	ch, err := loader.Load(chartPath)
	return ch, digest, err
}

func configureChartSource(options *action.ChartPathOptions, repoURL, version string) {
	options.RepoURL = repoURL
	options.Version = version
	// Kubby authenticates the exact previewed artifact with its own SHA-256 and
	// does not expose Helm's legacy OpenPGP provenance path. Keep this explicit:
	// golang.org/x/crypto/openpgp is deprecated and has no fixed release.
	options.Verify = false
}

func verifyChartDigest(expected, actual string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	actual = strings.ToLower(strings.TrimSpace(actual))
	if expected == "" {
		return fmt.Errorf("chart preview is required before install")
	}
	if len(expected) != len(actual) || subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return fmt.Errorf("the chart changed after preview; refusing install (preview %s, resolved %s)", expected, actual)
	}
	return nil
}

// ChartDefaultValues downloads a chart from its repo and returns its raw
// values.yaml — the authoritative defaults to pre-fill the install editor.
// Needs no cluster connection (only internet to the chart repo).
func ChartDefaultValues(repoURL, chartName, version string) (string, error) {
	cpo := action.ChartPathOptions{}
	configureChartSource(&cpo, repoURL, version)
	chartPath, err := cpo.LocateChart(chartName, cli.New())
	if err != nil {
		return "", err
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return "", err
	}
	for _, f := range ch.Raw {
		if f.Name == "values.yaml" {
			return string(f.Data), nil
		}
	}
	b, _ := yaml.Marshal(ch.Values)
	return string(b), nil
}

// HelmInstall installs a chart from a repository URL into the cluster.
func HelmInstall(c *Cluster, namespace, releaseName, repoURL, chartName, version, valuesYAML, expectedDigest string) error {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return err
	}
	inst := action.NewInstall(cfg)
	inst.ReleaseName = releaseName
	inst.Namespace = namespace
	inst.CreateNamespace = true
	ch, digest, err := locateAndLoadChart(inst, repoURL, chartName, version)
	if err != nil {
		return err
	}
	if err := verifyChartDigest(expectedDigest, digest); err != nil {
		return err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return err
	}
	_, err = inst.Run(ch, vals)
	return err
}

// HelmDiff carries the rendered manifest before/after an operation so the UI
// can show a line diff (helm-diff style) before the user commits.
type HelmDiff struct {
	Current     string `json:"current"`     // "" for a fresh install
	Proposed    string `json:"proposed"`    // rendered manifest that WOULD be applied
	ChartDigest string `json:"chartDigest"` // non-empty for install previews
}

// HelmInstallPreview renders (dry-run) the manifest a fresh install would create,
// without touching the cluster.
func HelmInstallPreview(c *Cluster, namespace, releaseName, repoURL, chartName, version, valuesYAML string) (*HelmDiff, error) {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return nil, err
	}
	inst := action.NewInstall(cfg)
	inst.ReleaseName = releaseName
	if inst.ReleaseName == "" {
		inst.ReleaseName = "preview"
	}
	inst.Namespace = namespace
	inst.DryRun = true
	inst.ClientOnly = false // talk to the apiserver for capabilities/version
	ch, digest, err := locateAndLoadChart(inst, repoURL, chartName, version)
	if err != nil {
		return nil, err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return nil, err
	}
	rel, err := inst.Run(ch, vals)
	if err != nil {
		return nil, err
	}
	return &HelmDiff{Current: "", Proposed: rel.Manifest, ChartDigest: digest}, nil
}

// HelmUpgradePreview renders (dry-run) the manifest an upgrade-values would
// produce and returns it alongside the current manifest for diffing.
func HelmUpgradePreview(c *Cluster, namespace, name, valuesYAML string) (*HelmDiff, error) {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return nil, err
	}
	current, err := action.NewGet(cfg).Run(name)
	if err != nil {
		return nil, err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return nil, err
	}
	up := action.NewUpgrade(cfg)
	up.Namespace = namespace
	up.DryRun = true
	proposed, err := up.Run(name, current.Chart, vals)
	if err != nil {
		return nil, err
	}
	return &HelmDiff{Current: current.Manifest, Proposed: proposed.Manifest}, nil
}

// HelmGetRevision returns the detail (manifest/values/notes) of one specific
// revision — used to diff two history points or preview a rollback target.
func HelmGetRevision(c *Cluster, namespace, name string, revision int) (*HelmReleaseDetail, error) {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return nil, err
	}
	get := action.NewGet(cfg)
	get.Version = revision
	rel, err := get.Run(name)
	if err != nil {
		return nil, err
	}
	valuesYAML, _ := yaml.Marshal(rel.Config)
	chartName := ""
	appVer := ""
	if rel.Chart != nil && rel.Chart.Metadata != nil {
		chartName = rel.Chart.Metadata.Name + "-" + rel.Chart.Metadata.Version
		appVer = rel.Chart.Metadata.AppVersion
	}
	return &HelmReleaseDetail{
		Name: rel.Name, Namespace: rel.Namespace, Revision: rel.Version,
		Status: rel.Info.Status.String(), Chart: chartName, AppVersion: appVer,
		Notes: rel.Info.Notes, Values: string(valuesYAML), Manifest: rel.Manifest,
	}, nil
}

// HelmResource is one Kubernetes object owned by a release, with live health.
type HelmResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"` // e.g. "1/1", "Running", "-"
	Ready     bool   `json:"ready"`
}

// HelmReleaseResources parses a release's manifest into the objects it owns and
// fetches live readiness for the common workload kinds.
func HelmReleaseResources(ctx context.Context, c *Cluster, namespace, name string) ([]HelmResource, error) {
	detail, err := HelmGet(c, namespace, name)
	if err != nil {
		return nil, err
	}
	var out []HelmResource
	for _, doc := range strings.Split(detail.Manifest, "\n---") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var head struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
		}
		if err := yaml.Unmarshal([]byte(doc), &head); err != nil || head.Kind == "" || head.Metadata.Name == "" {
			continue
		}
		ns := head.Metadata.Namespace
		if ns == "" {
			ns = namespace
		}
		res := HelmResource{Kind: head.Kind, Name: head.Metadata.Name, Namespace: ns, Status: "-"}
		res.Status, res.Ready = resourceHealth(ctx, c, head.Kind, ns, head.Metadata.Name)
		out = append(out, res)
	}
	return out, nil
}

// resourceHealth returns a short live status for a workload object; unknown
// kinds report "-".
func resourceHealth(ctx context.Context, c *Cluster, kind, ns, name string) (string, bool) {
	switch kind {
	case "Deployment":
		d, err := c.Clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "missing", false
		}
		return fmt.Sprintf("%d/%d ready", d.Status.ReadyReplicas, d.Status.Replicas), d.Status.ReadyReplicas == d.Status.Replicas && d.Status.Replicas > 0
	case "StatefulSet":
		s, err := c.Clientset.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "missing", false
		}
		return fmt.Sprintf("%d/%d ready", s.Status.ReadyReplicas, s.Status.Replicas), s.Status.ReadyReplicas == s.Status.Replicas && s.Status.Replicas > 0
	case "DaemonSet":
		ds, err := c.Clientset.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "missing", false
		}
		return fmt.Sprintf("%d/%d ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled), ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0
	case "Pod":
		p, err := c.Clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "missing", false
		}
		return string(p.Status.Phase), p.Status.Phase == "Running" || p.Status.Phase == "Succeeded"
	case "Job":
		j, err := c.Clientset.BatchV1().Jobs(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "missing", false
		}
		return fmt.Sprintf("%d succeeded", j.Status.Succeeded), j.Status.Succeeded > 0
	default:
		return "-", true
	}
}

// HelmTest runs a release's test hooks and reports each hook's outcome.
func HelmTest(c *Cluster, namespace, name string) (string, error) {
	cfg, err := newHelmConfig(c, namespace)
	if err != nil {
		return "", err
	}
	t := action.NewReleaseTesting(cfg)
	t.Namespace = namespace
	rel, runErr := t.Run(name)
	if rel == nil {
		if runErr != nil {
			return "", runErr
		}
		return "No test hooks defined for this release.", nil
	}
	var lines []string
	for _, h := range rel.Hooks {
		isTest := false
		for _, e := range h.Events {
			if e == release.HookTest {
				isTest = true
			}
		}
		if !isTest {
			continue
		}
		phase := "unknown"
		if h.LastRun.Phase != "" {
			phase = string(h.LastRun.Phase)
		}
		lines = append(lines, fmt.Sprintf("%s: %s", h.Name, phase))
	}
	if len(lines) == 0 {
		if runErr != nil {
			return "", runErr
		}
		return "No test hooks defined for this release.", nil
	}
	result := strings.Join(lines, "\n")
	if runErr != nil {
		return result, runErr
	}
	return result, nil
}
