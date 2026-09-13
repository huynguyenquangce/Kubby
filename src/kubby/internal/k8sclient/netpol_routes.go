package k8sclient

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Routed traffic reaches a Service's Pods from the entry point's own Pods: the
// ingress controller for an Ingress, the ingress-gateway for an Istio Gateway.
// Evaluating NetworkPolicy from those Pods turns "isolated" into the answer an
// operator wants — "this route is blocked by policy X". The source is never
// guessed: a controller Kubby cannot map to its Pods is reported as not
// evaluated, and the hop keeps its routing verdict.

type ingressControllerPods struct {
	label    string
	selector string
}

// knownIngressControllers maps an IngressClass spec.controller to the labels
// the controller's official Helm chart puts on its Pods.
var knownIngressControllers = map[string]ingressControllerPods{
	"k8s.io/ingress-nginx":          {"ingress-nginx controller", "app.kubernetes.io/name=ingress-nginx,app.kubernetes.io/component=controller"},
	"traefik.io/ingress-controller": {"Traefik", "app.kubernetes.io/name=traefik"},
}

const defaultIngressClassAnnotation = "ingressclass.kubernetes.io/is-default-class"

type entryEvaluator struct {
	ctx         context.Context
	c           *Cluster
	scope       string
	flows       *NetworkFlows
	pods        map[string]*corev1.Pod
	services    map[string]*corev1.Service
	scoped      map[string][]*compiledNetworkPolicy // compiled from the view's own List
	loaded      map[string][]*compiledNetworkPolicy // other namespaces, listed on demand
	failed      map[string]bool
	labelsFor   func(string) (map[string]string, bool)
	limitations []string

	classesLoaded  bool
	classes        []netv1.IngressClass
	classesErr     error
	controllerPods map[string][]corev1.Pod
	controllerErr  map[string]error
}

// evaluateEntryPolicies sets FlowService.EntryPolicy on routed hops and
// FlowIngress.EntryNote where the source cannot be identified. It runs only
// when the view's scope has NetworkPolicies: without one no destination Pod is
// isolated, so a Traffic refresh on a cluster without policies costs nothing.
// Egress isolation of the controller's own namespace is therefore not evaluated
// in a scoped view whose namespace has no policy.
func evaluateEntryPolicies(ctx context.Context, c *Cluster, scope string, flows *NetworkFlows, pods []corev1.Pod, services []corev1.Service, policies []netv1.NetworkPolicy) {
	if flows.PolicyCount == 0 || len(flows.Ingresses) == 0 {
		return
	}
	scoped, _ := compileNetworkPolicies(policies)
	e := &entryEvaluator{
		ctx: ctx, c: c, scope: scope, flows: flows,
		pods:     make(map[string]*corev1.Pod, len(pods)),
		services: make(map[string]*corev1.Service, len(services)),
		scoped:   scoped, loaded: map[string][]*compiledNetworkPolicy{}, failed: map[string]bool{},
		controllerPods: map[string][]corev1.Pod{}, controllerErr: map[string]error{},
	}
	e.labelsFor = newNamespaceLabelLookup(ctx, c, &e.limitations)
	for i := range pods {
		e.pods[pods[i].Namespace+"/"+pods[i].Name] = &pods[i]
	}
	for i := range services {
		e.services[services[i].Namespace+"/"+services[i].Name] = &services[i]
	}

	for i := range flows.Ingresses {
		fi := &flows.Ingresses[i]
		if !hasRoutedPods(fi) {
			continue
		}
		sources, label, note := e.entrySource(fi)
		if note != "" {
			fi.EntryNote = note
			continue
		}
		if len(sources) == 0 {
			continue // a Gateway with no gateway Pod already carries that warning
		}
		source := fmt.Sprintf("%s (%s)", label, podGroupText(sources))
		for j := range fi.Services {
			e.evaluateHop(&fi.Services[j], sources, label, source)
		}
	}
	for _, limitation := range dedupe(e.limitations) {
		flows.Warnings = append(flows.Warnings, limitation)
	}
}

func hasRoutedPods(fi *FlowIngress) bool {
	for _, svc := range fi.Services {
		if len(svc.Pods) > 0 {
			return true
		}
	}
	return false
}

// entrySource returns the Pods that forward an entry point's traffic, a short
// label for them, or a note explaining why they cannot be identified.
func (e *entryEvaluator) entrySource(fi *FlowIngress) ([]corev1.Pod, string, string) {
	if fi.Kind == "Gateway" {
		return fi.entryPods, "Istio ingress gateway", ""
	}
	const skipped = "NetworkPolicy on these routes was not evaluated — use Check traffic from the controller Pod."
	class, err := e.ingressClass(fi.Class)
	switch {
	case err != nil:
		return nil, "", fmt.Sprintf("IngressClasses could not be read (%v); %s", err, skipped)
	case class == nil && fi.Class == "":
		return nil, "", "This Ingress names no IngressClass and none is marked default, so its controller is unknown; " + skipped
	case class == nil:
		return nil, "", fmt.Sprintf("IngressClass %q was not found, so its controller is unknown; %s", fi.Class, skipped)
	}
	known, ok := knownIngressControllers[class.Spec.Controller]
	if !ok {
		return nil, "", fmt.Sprintf("IngressClass %q uses controller %q, which Kubby cannot map to its Pods; %s", class.Name, class.Spec.Controller, skipped)
	}
	pods, err := e.podsForController(known.selector)
	switch {
	case err != nil:
		return nil, "", fmt.Sprintf("%s Pods could not be listed (%v); %s", known.label, err, skipped)
	case len(pods) == 0:
		return nil, "", fmt.Sprintf("No %s Pods were found (labels %s); %s", known.label, known.selector, skipped)
	}
	return pods, known.label, ""
}

// ingressClass resolves a class name, or the cluster default when name is
// empty. The class list is read at most once per Traffic load.
func (e *entryEvaluator) ingressClass(name string) (*netv1.IngressClass, error) {
	if !e.classesLoaded {
		e.classesLoaded = true
		list, err := e.c.Clientset.NetworkingV1().IngressClasses().List(e.ctx, metav1.ListOptions{})
		if err != nil {
			e.classesErr = err
		} else {
			e.classes = list.Items
		}
	}
	if e.classesErr != nil {
		return nil, e.classesErr
	}
	for i := range e.classes {
		class := &e.classes[i]
		if (name != "" && class.Name == name) || (name == "" && class.Annotations[defaultIngressClassAnnotation] == "true") {
			return class, nil
		}
	}
	return nil, nil
}

func (e *entryEvaluator) podsForController(selector string) ([]corev1.Pod, error) {
	if pods, ok := e.controllerPods[selector]; ok {
		return pods, e.controllerErr[selector]
	}
	list, err := e.c.Clientset.CoreV1().Pods("").List(e.ctx, metav1.ListOptions{LabelSelector: selector})
	pods := []corev1.Pod{}
	if err == nil {
		for i := range list.Items {
			if list.Items[i].DeletionTimestamp == nil {
				pods = append(pods, list.Items[i])
			}
		}
	}
	e.controllerPods[selector], e.controllerErr[selector] = pods, err
	return pods, err
}

// policiesFor returns a namespace's compiled policies, listing namespaces the
// view did not already cover. ok is false when they could not be read.
func (e *entryEvaluator) policiesFor(namespace string) ([]*compiledNetworkPolicy, bool) {
	if e.scope == "" || namespace == e.scope {
		return e.scoped[namespace], true
	}
	if policies, ok := e.loaded[namespace]; ok {
		return policies, true
	}
	if e.failed[namespace] {
		return nil, false
	}
	policies, err := loadCompiledPolicies(e.ctx, e.c, namespace)
	if err != nil {
		e.failed[namespace] = true
		e.limitations = append(e.limitations, fmt.Sprintf("network policies in %s: %v; routes involving it were not evaluated", namespace, err))
		return nil, false
	}
	e.loaded[namespace] = policies
	return policies, true
}

// evaluateHop decides, for each Pod behind a routed Service, whether any entry
// Pod may open a connection to it on a routed port. A blocked or partially
// blocked hop gains a warning and counts as a broken path; an allowed hop keeps
// the verdict only when policy was involved, so unpolicied routes stay quiet.
func (e *entryEvaluator) evaluateHop(fs *FlowService, sources []corev1.Pod, label, source string) {
	svc := e.services[fs.Namespace+"/"+fs.Name]
	if svc == nil || len(fs.Pods) == 0 {
		return // out of scope, external or missing: nothing reliable to evaluate
	}
	destinationPolicies, ok := e.policiesFor(fs.Namespace)
	if !ok {
		return
	}
	sourceEgress := make([][]*compiledNetworkPolicy, len(sources))
	for i := range sources {
		policies, ok := e.policiesFor(sources[i].Namespace)
		if !ok {
			return
		}
		sourceEgress[i] = selectingPolicies(policies, &sources[i], netv1.PolicyTypeEgress)
	}
	ports := routedServicePorts(svc, fs.routePorts)

	verdict := &FlowEntryPolicy{Source: source, Policies: []string{}}
	blocking, admitting := map[string]bool{}, map[string]bool{}
	isolated := false
	for _, flowPod := range fs.Pods {
		destination := e.pods[flowPod.Namespace+"/"+flowPod.Name]
		if destination == nil {
			continue
		}
		verdict.Total++
		destinationIngress := selectingPolicies(destinationPolicies, destination, netv1.PolicyTypeIngress)
		isolated = isolated || len(destinationIngress) > 0
		reachable := false
		for i := range sources {
			isolated = isolated || len(sourceEgress[i]) > 0
			for _, servicePort := range ports {
				port := serviceTargetPort(destination, servicePort)
				egressOK, egressBy := directionAllows(netv1.PolicyTypeEgress, sourceEgress[i], destination, port, e.labelsFor)
				ingressOK, ingressBy := directionAllows(netv1.PolicyTypeIngress, destinationIngress, &sources[i], port, e.labelsFor)
				if egressOK && ingressOK {
					reachable = true
					for _, name := range append(egressBy, ingressBy...) {
						admitting[name] = true
					}
					break
				}
				if !egressOK {
					for _, policy := range sourceEgress[i] {
						blocking[policy.ref.Name] = true
					}
				}
				if !ingressOK {
					for _, policy := range destinationIngress {
						blocking[policy.ref.Name] = true
					}
				}
			}
			if reachable {
				break
			}
		}
		if !reachable {
			verdict.Blocked++
		}
	}
	if verdict.Total == 0 || !isolated {
		return
	}

	switch {
	case verdict.Blocked == 0:
		verdict.Verdict = TrafficAllowed
		verdict.Policies = sortedKeys(admitting)
	case verdict.Blocked == verdict.Total:
		verdict.Verdict = TrafficBlocked
		verdict.Policies = sortedKeys(blocking)
	default:
		verdict.Verdict = TrafficPartial
		verdict.Policies = sortedKeys(blocking)
	}
	fs.EntryPolicy = verdict
	if verdict.Blocked == 0 {
		return
	}
	message := fmt.Sprintf("NetworkPolicy blocks the %s from every Pod (%s)", label, strings.Join(verdict.Policies, ", "))
	if verdict.Verdict == TrafficPartial {
		message = fmt.Sprintf("NetworkPolicy blocks the %s from %d of %d Pods (%s) — requests fail intermittently", label, verdict.Blocked, verdict.Total, strings.Join(verdict.Policies, ", "))
	}
	if fs.Warning == "" {
		e.flows.BrokenCount++
		fs.Warning = message
	} else {
		fs.Warning += " · " + message
	}
}

// directionAllows is the verdict of directionVerdict without its explanation,
// for bulk evaluation across many routed Pods.
func directionAllows(direction netv1.PolicyType, selecting []*compiledNetworkPolicy, peer *corev1.Pod, port destinationPort, labelsFor func(string) (map[string]string, bool)) (bool, []string) {
	if len(selecting) == 0 {
		return true, nil
	}
	var allowing []string
	for _, policy := range selecting {
		if policy.allows(direction, peer, port, labelsFor) {
			allowing = append(allowing, policy.ref.Name)
		}
	}
	return len(allowing) > 0, allowing
}

// routedServicePorts returns the Service ports a route targets. Without a port
// on the route every Service port is considered, and a hop counts as reachable
// when any of them is — the route could be using that one.
func routedServicePorts(svc *corev1.Service, routePorts []intstr.IntOrString) []corev1.ServicePort {
	chosen := []corev1.ServicePort{}
	for _, routePort := range routePorts {
		for _, servicePort := range svc.Spec.Ports {
			if (routePort.Type == intstr.Int && servicePort.Port == routePort.IntVal) ||
				(routePort.Type == intstr.String && servicePort.Name == routePort.StrVal) {
				chosen = append(chosen, servicePort)
				break
			}
		}
	}
	if len(chosen) == 0 {
		return svc.Spec.Ports
	}
	return chosen
}

// serviceBackendPortRef keeps an Ingress backend port as its number or name.
func serviceBackendPortRef(port netv1.ServiceBackendPort) intstr.IntOrString {
	if port.Name != "" {
		return intstr.FromString(port.Name)
	}
	return intstr.FromInt32(port.Number)
}

func podGroupText(pods []corev1.Pod) string {
	namespaces := map[string]bool{}
	for _, pod := range pods {
		namespaces[pod.Namespace] = true
	}
	noun := "Pods"
	if len(pods) == 1 {
		noun = "Pod"
	}
	return fmt.Sprintf("%d %s in %s", len(pods), noun, strings.Join(sortedKeys(namespaces), ", "))
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
