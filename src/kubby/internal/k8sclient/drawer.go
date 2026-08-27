package k8sclient

import (
	"context"
	"sync"
)

// DrawerSnapshot is the initial evidence for a resource Details tab. Detail is
// required; Events and the supported relationship tree are best-effort sections
// so RBAC on one API cannot blank the drawer.
type DrawerSnapshot struct {
	Detail        *ResourceDetail   `json:"detail"`
	Events        []EventInfo       `json:"events"`
	Relation      *RelationNode     `json:"relation"`
	NodePods      []PodInfo         `json:"nodePods"`
	NamespaceInfo []NsKindCount     `json:"namespaceInfo"`
	SectionErrors map[string]string `json:"sectionErrors"`
}

// GetDrawerSnapshot overlaps independent Kubernetes reads behind one Wails
// call. Secret reveal remains separate because it is an explicit sensitive
// action.
func GetDrawerSnapshot(ctx context.Context, c *Cluster, kind, namespace, name string) (*DrawerSnapshot, error) {
	out := &DrawerSnapshot{SectionErrors: map[string]string{}}
	var detailErr error
	var mu sync.Mutex
	var wg sync.WaitGroup

	run := func(section string, task func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := task(); err != nil {
				mu.Lock()
				out.SectionErrors[section] = err.Error()
				mu.Unlock()
			}
		}()
	}

	run("detail", func() error {
		out.Detail, detailErr = GetDetail(ctx, c, kind, namespace, name)
		return detailErr
	})
	run("events", func() error {
		var err error
		out.Events, err = ListEvents(ctx, c, kind, namespace, name)
		return err
	})

	var relationTask func() (*RelationNode, error)
	switch bareKind(kind) {
	case "Deployment":
		relationTask = func() (*RelationNode, error) { return DeploymentTree(ctx, c, namespace, name) }
	case "Service":
		relationTask = func() (*RelationNode, error) { return ServiceTree(ctx, c, namespace, name) }
	case "Ingress":
		relationTask = func() (*RelationNode, error) { return IngressTree(ctx, c, namespace, name) }
	}
	if relationTask != nil {
		run("relations", func() error {
			var err error
			out.Relation, err = relationTask()
			return err
		})
	}
	switch bareKind(kind) {
	case "Node":
		run("nodePods", func() error {
			var err error
			out.NodePods, err = PodsOnNode(ctx, c, name)
			return err
		})
	case "Namespace":
		run("namespaceInfo", func() error {
			var err error
			out.NamespaceInfo, err = NamespaceSummary(ctx, c, name)
			return err
		})
	}

	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if detailErr != nil {
		return nil, detailErr
	}
	delete(out.SectionErrors, "detail")
	return out, nil
}
