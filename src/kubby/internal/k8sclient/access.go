package k8sclient

import (
	"context"
	"sync"

	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessSet is what the current kubeconfig's identity may do to one kind in one
// namespace, as the cluster itself reports it via SelfSubjectAccessReview.
//
// It exists so the UI can stop offering actions that will fail: before this, a
// read-only token still got a Delete menu item, and found out at the 403.
type AccessSet struct {
	Kind      string          `json:"kind"`
	Namespace string          `json:"namespace"` // "" for a cluster-scoped kind
	Verbs     map[string]bool `json:"verbs"`

	// Checked is false when the cluster could not answer — an old apiserver, a
	// proxy in the way, a network blip.
	//
	// **An unanswered question is not a "no".** When Checked is false every verb
	// is reported as allowed, because greying out the whole UI on a failed probe
	// would be a far worse bug than letting a click fail at the API, which is
	// exactly what happened before this existed.
	Checked bool `json:"checked"`
}

// Allowed is the nil-safe read the UI wants: no answer means do not restrict.
func (a *AccessSet) Allowed(verb string) bool {
	if a == nil || a.Verbs == nil {
		return true
	}
	v, ok := a.Verbs[verb]
	return !ok || v
}

// accessProbe is one question asked of the API. `key` is the name the UI uses,
// which is not always the verb: "exec" is a *create* on the pods/exec
// subresource, and no amount of `get` on pods implies it.
type accessProbe struct {
	key         string
	verb        string
	subresource string
}

const accessConcurrency = 8

func probesFor(ak APIKind) []accessProbe {
	probes := []accessProbe{
		{"get", "get", ""},
		{"list", "list", ""},
		{"create", "create", ""},
		{"update", "update", ""},
		{"patch", "patch", ""},
		{"delete", "delete", ""},
	}
	// The three Pod subresources Kubby exposes as separate features. Each is
	// governed independently of the Pod itself — a read-only role that can also
	// exec is a common (and dangerous) combination, and so is the reverse.
	if ak.GVR.Group == "" && ak.GVR.Resource == "pods" {
		probes = append(probes,
			accessProbe{"logs", "get", "log"},
			accessProbe{"exec", "create", "exec"},
			accessProbe{"portforward", "create", "portforward"},
		)
	}
	// Scaling is an update on deployments/scale, which can be granted or denied
	// independently of updates to the Deployment object itself.
	if ak.GVR.Group == "apps" && ak.GVR.Resource == "deployments" {
		probes = append(probes, accessProbe{"scale", "update", "scale"})
	}
	return probes
}

// CanI asks the cluster what this token may do to `kind` in `namespace`.
//
// Results are cached per cluster connection: RBAC does not usually change while
// the app is open, and a stale *allow* is harmless because the apiserver still
// enforces the real answer. A failed probe is never cached, so a blip does not
// stick for the session.
func CanI(ctx context.Context, c *Cluster, kind, namespace string) (*AccessSet, error) {
	ak, err := c.ResolveKind(kind)
	if err != nil {
		return nil, err
	}
	if !ak.Namespaced {
		namespace = "" // a namespace on a cluster-scoped check is meaningless
	}

	cacheKey := kind + "|" + namespace
	c.accessMu.Lock()
	cached, ok := c.access[cacheKey]
	c.accessMu.Unlock()
	if ok {
		return cached, nil
	}

	probes := probesFor(ak)
	verbs := make(map[string]bool, len(probes))
	failed := 0

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, accessConcurrency)
	for _, p := range probes {
		wg.Add(1)
		go func(p accessProbe) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			allowed, err := c.canI(ctx, ak.GVR.Group, ak.GVR.Resource, p.subresource, p.verb, namespace)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				verbs[p.key] = true // see AccessSet.Checked
				return
			}
			verbs[p.key] = allowed
		}(p)
	}
	wg.Wait()

	set := &AccessSet{Kind: kind, Namespace: namespace, Verbs: verbs, Checked: failed == 0}
	if set.Checked {
		c.accessMu.Lock()
		if c.access == nil {
			c.access = map[string]*AccessSet{}
		}
		c.access[cacheKey] = set
		c.accessMu.Unlock()
	}
	return set, nil
}

func (c *Cluster) canI(ctx context.Context, group, resource, subresource, verb, namespace string) (bool, error) {
	review := &authv1.SelfSubjectAccessReview{
		Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace:   namespace,
				Group:       group,
				Resource:    resource,
				Subresource: subresource,
				Verb:        verb,
			},
		},
	}
	res, err := c.Clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return res.Status.Allowed, nil
}
