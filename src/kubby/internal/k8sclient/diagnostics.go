package k8sclient

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
)

// ClusterDiag is what Kubby can say about the cluster it is talking to. It exists
// so a bug report can answer the questions that otherwise take three messages to
// establish: which Kubernetes version, is metrics-server there, does the token
// actually work.
type ClusterDiag struct {
	Context    string `json:"context"`    // the kubeconfig context name
	Endpoint   string `json:"endpoint"`   // API server URL
	Version    string `json:"version"`    // server GitVersion, e.g. v1.36.1
	Platform   string `json:"platform"`   // server GOOS/GOARCH
	APIGroups  int    `json:"apiGroups"`  // how many groups discovery returned
	CRDKinds   int    `json:"crdKinds"`   // how many CRD-defined kinds are served
	Metrics    string `json:"metrics"`    // "available" | "not available" | why not
	Nodes      int    `json:"nodes"`      // -1 when the list was refused
	Reachable  bool   `json:"reachable"`  // false means the rest is unreliable
	FirstError string `json:"firstError"` // the first thing that failed, if anything did
}

// Diagnose probes the connected cluster. Every probe is best-effort: a forbidden
// or missing capability is *information*, not a failure, so the report is
// produced even from a half-working connection — that is exactly when it is most
// wanted.
func Diagnose(ctx context.Context, c *Cluster, contextName string) *ClusterDiag {
	d := &ClusterDiag{Context: contextName, Nodes: -1, Metrics: "not available"}
	if c == nil {
		d.FirstError = "no cluster is connected"
		return d
	}
	if c.Rest != nil {
		d.Endpoint = c.Rest.Host
	}

	note := func(err error) {
		if err != nil && d.FirstError == "" {
			d.FirstError = err.Error()
		}
	}

	if c.Discovery != nil {
		if v, err := c.Discovery.ServerVersion(); err == nil {
			d.Reachable = true
			d.Version = v.GitVersion
			d.Platform = v.Platform
		} else {
			note(err)
		}
		// A partial discovery failure still tells us the group count, and is worth
		// reporting verbatim — a stale aggregated APIService is a common cause of
		// otherwise baffling behaviour elsewhere in the app.
		groups, _, err := c.Discovery.ServerGroupsAndResources()
		d.APIGroups = len(groups)
		if err != nil && discovery.IsGroupDiscoveryFailedError(err) {
			note(fmt.Errorf("partial API discovery: %w", err))
		} else {
			note(err)
		}
	}

	if nodes, err := ListNodes(ctx, c.Clientset); err == nil {
		d.Nodes = len(nodes)
	} else {
		note(err)
	}

	switch {
	case c.Metrics == nil:
		d.Metrics = "no client (metrics.k8s.io unavailable at connect)"
	default:
		if m, err := NodeMetrics(ctx, c); err != nil {
			d.Metrics = "error: " + err.Error()
		} else if m == nil {
			d.Metrics = "not available (metrics-server not installed)"
		} else {
			d.Metrics = fmt.Sprintf("available (%d node(s) reporting)", len(m))
		}
	}

	if custom, err := CustomKinds(ctx, c); err == nil {
		d.CRDKinds = custom.Total
	} else {
		note(err)
	}
	return d
}
