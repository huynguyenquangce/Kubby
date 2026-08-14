// kubby-cli là công cụ phụ dùng chung logic k8sclient với app Kubby chính (FR-8) —
// tiện cho debug nhanh không cần mở UI, không phải kênh dùng chính.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"kubby/internal/buildinfo"
	"kubby/internal/k8sclient"

	"github.com/spf13/cobra"
)

var kubeconfigPath string
var kubeContext string

func main() {
	home, _ := os.UserHomeDir()
	defaultKubeconfig := filepath.Join(home, ".kube", "config")
	if v := os.Getenv("KUBECONFIG"); v != "" {
		defaultKubeconfig = v
	}

	root := &cobra.Command{
		Use:     "kubby-cli",
		Short:   "Công cụ debug nhanh cho Kubby, không cần mở UI",
		Version: buildinfo.Get().String(), // gives `kubby-cli --version` / `-v`
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.PersistentFlags().StringVar(&kubeconfigPath, "kubeconfig", defaultKubeconfig, "đường dẫn tới file kubeconfig")
	root.PersistentFlags().StringVar(&kubeContext, "context", "", "context muốn dùng (mặc định: current-context)")

	// diagnostics — the same probe the GUI's "Copy diagnostics" button runs.
	diagnosticsCmd := &cobra.Command{
		Use:   "diagnostics",
		Short: "In báo cáo môi trường + cluster (dùng khi báo lỗi)",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := buildinfo.Get()
			fmt.Println(info.String())
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				fmt.Printf("cluster: không kết nối được — %v\n", err)
				return nil // a failed connection IS the diagnosis; don't exit non-zero
			}
			d := k8sclient.Diagnose(context.Background(), cluster, kubeContext)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "reachable\t%v\n", d.Reachable)
			fmt.Fprintf(w, "endpoint\t%s\n", d.Endpoint)
			fmt.Fprintf(w, "server\t%s (%s)\n", d.Version, d.Platform)
			fmt.Fprintf(w, "nodes\t%d\n", d.Nodes)
			fmt.Fprintf(w, "api groups\t%d\n", d.APIGroups)
			fmt.Fprintf(w, "CRD kinds\t%d\n", d.CRDKinds)
			fmt.Fprintf(w, "metrics\t%s\n", d.Metrics)
			if d.FirstError != "" {
				fmt.Fprintf(w, "first probe error\t%s\n", d.FirstError)
			}
			return w.Flush()
		},
	}
	root.AddCommand(diagnosticsCmd)

	// overview — the same single backend snapshot used by the dashboard.
	overviewCmd := &cobra.Command{
		Use:   "overview",
		Short: "Đo và in snapshot của Overview (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			started := time.Now()
			snapshot, err := k8sclient.OverviewSnapshot(context.Background(), cluster)
			if err != nil {
				return err
			}
			fmt.Printf("%d nodes, %d namespaces, %d pods, %d deployments, %d failing · %d top pods, %d events · %s\n",
				snapshot.Stats.Nodes, snapshot.Stats.Namespaces, snapshot.Stats.Pods,
				snapshot.Stats.Deployments, snapshot.Stats.Errors, len(snapshot.TopPods),
				len(snapshot.Events), time.Since(started).Round(time.Millisecond))
			for _, warning := range snapshot.Warnings {
				fmt.Printf("warning: %s\n", warning)
			}
			return nil
		},
	}
	root.AddCommand(overviewCmd)

	var structureNamespace string
	structureCmd := &cobra.Command{
		Use:   "structure",
		Short: "Kiểm tra topology Entry point → Service → Workload → Pod (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			started := time.Now()
			structure, err := k8sclient.ClusterStructure(context.Background(), cluster, structureNamespace)
			if err != nil {
				return err
			}
			fmt.Printf("%d entry points, %d internal services, %d unexposed workloads · %d pods, %d unhealthy · %s\n",
				len(structure.Entries), len(structure.Internal), len(structure.Unexposed),
				structure.Summary.Pods, structure.Summary.Unhealthy, time.Since(started).Round(time.Millisecond))
			for _, warning := range structure.Warnings {
				fmt.Printf("warning: %s\n", warning)
			}
			return nil
		},
	}
	structureCmd.Flags().StringVarP(&structureNamespace, "namespace", "n", "", "namespace (rỗng = toàn cluster)")
	root.AddCommand(structureCmd)

	getCmd := &cobra.Command{Use: "get", Short: "Xem resource trong cluster"}

	var podsNamespace string
	getPods := &cobra.Command{
		Use:   "pods",
		Short: "Liệt kê pod (namespace rỗng = toàn cluster)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			pods, err := k8sclient.ListPods(context.Background(), cluster.Clientset, podsNamespace)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAMESPACE\tNAME\tSTATUS\tRESTARTS\tIP\tNODE\tAGE")
			for _, p := range pods {
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", p.Namespace, p.Name, p.Status, p.Restarts, p.PodIP, p.Node, p.Age)
			}
			return w.Flush()
		},
	}
	getPods.Flags().StringVarP(&podsNamespace, "namespace", "n", "", "namespace (rỗng = tất cả)")

	getNamespaces := &cobra.Command{
		Use:     "namespaces",
		Aliases: []string{"ns"},
		Short:   "Liệt kê namespace",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			namespaces, err := k8sclient.ListNamespaces(context.Background(), cluster.Clientset)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS")
			for _, ns := range namespaces {
				fmt.Fprintf(w, "%s\t%s\n", ns.Name, ns.Status)
			}
			return w.Flush()
		},
	}

	getCmd.AddCommand(getPods, getNamespaces)
	root.AddCommand(getCmd)

	var yamlNamespace string
	yamlCmd := &cobra.Command{
		Use:   "yaml <kind> <name>",
		Short: "In YAML của một resource (Pod/Deployment/Service/Namespace/Node)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			out, err := k8sclient.GetYAML(context.Background(), cluster, args[0], yamlNamespace, args[1])
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	yamlCmd.Flags().StringVarP(&yamlNamespace, "namespace", "n", "default", "namespace")
	root.AddCommand(yamlCmd)

	var logsNamespace, logsContainer string
	var logsTail int
	logsCmd := &cobra.Command{
		Use:   "logs <pod>",
		Short: "In log gần nhất của một pod",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			out, err := k8sclient.PodLogs(context.Background(), cluster, logsNamespace, args[0], logsContainer, int64(logsTail))
			if err != nil {
				return err
			}
			fmt.Print(out)
			return nil
		},
	}
	logsCmd.Flags().StringVarP(&logsNamespace, "namespace", "n", "default", "namespace")
	logsCmd.Flags().StringVarP(&logsContainer, "container", "c", "", "container (mặc định: container đầu)")
	logsCmd.Flags().IntVar(&logsTail, "tail", 200, "số dòng cuối")
	root.AddCommand(logsCmd)

	var applyFile string
	applyCmd := &cobra.Command{
		Use:   "apply -f <file>",
		Short: "Áp dụng YAML (create-or-update, giống Import YAML / nút Create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(applyFile)
			if err != nil {
				return err
			}
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			report, err := k8sclient.ApplyYAML(context.Background(), cluster, string(data))
			if err != nil {
				return err
			}
			fmt.Println(report)
			return nil
		},
	}
	applyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "đường dẫn file YAML")
	_ = applyCmd.MarkFlagRequired("file")
	root.AddCommand(applyCmd)

	// diff — the dry-run preview behind Import YAML's Preview button. Changes nothing.
	var diffFile string
	diffCmd := &cobra.Command{
		Use:   "diff -f <file>",
		Short: "Xem YAML này sẽ đổi gì (server-side dry run, không ghi gì cả)",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(diffFile)
			if err != nil {
				return err
			}
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			d, err := k8sclient.ApplyPreview(context.Background(), cluster, string(data))
			if err != nil {
				return err
			}
			fmt.Printf("%d create, %d update, %d unchanged, %d cannot be previewed\n",
				d.Create, d.Update, d.Unchanged, d.Failed)
			for _, doc := range d.Docs {
				fmt.Printf("\n=== %s: %s\n", strings.ToUpper(doc.Action), doc.Ref)
				switch {
				case doc.Error != "":
					fmt.Printf("    %s\n", doc.Error)
				case doc.Action == "unchanged":
					// nothing to show
				default:
					for _, line := range unifiedDiff(doc.Current, doc.Proposed) {
						fmt.Println(line)
					}
				}
			}
			return nil
		},
	}
	diffCmd.Flags().StringVarP(&diffFile, "file", "f", "", "đường dẫn file YAML")
	_ = diffCmd.MarkFlagRequired("file")
	root.AddCommand(diffCmd)

	// can-i — what the current token may do to a kind (drives the UI's greying-out).
	var caniNs string
	caniCmd := &cobra.Command{
		Use:   "can-i <kind>",
		Short: "Token này được làm gì với kind đó (SelfSubjectAccessReview)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			set, err := k8sclient.CanI(context.Background(), cluster, args[0], caniNs)
			if err != nil {
				return err
			}
			where := set.Namespace
			if where == "" {
				where = "(cluster-scoped)"
			}
			fmt.Printf("%s in %s — answered by the cluster: %v\n", set.Kind, where, set.Checked)
			keys := make([]string, 0, len(set.Verbs))
			for k := range set.Verbs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, k := range keys {
				mark := "no"
				if set.Verbs[k] {
					mark = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\n", k, mark)
			}
			return w.Flush()
		},
	}
	caniCmd.Flags().StringVarP(&caniNs, "namespace", "n", "default", "namespace")
	root.AddCommand(caniCmd)

	// sizing — requests/limits vs real usage (the Right-sizing view, read-only).
	var sizingNs string
	sizingCmd := &cobra.Command{
		Use:   "sizing",
		Short: "So requests/limits với usage thật (namespace rỗng = cả cluster)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			r, err := k8sclient.Sizing(context.Background(), cluster, sizingNs)
			if err != nil {
				return err
			}
			if r.Note != "" {
				fmt.Println("note:", r.Note)
			}
			fmt.Printf("%d pods, %d containers · requests CPU %s / mem %s · usage CPU %s / mem %s\n",
				r.Totals.Pods, r.Totals.Containers,
				k8sclient.FormatCPU(r.Totals.CPURequest), k8sclient.FormatMem(r.Totals.MemRequest),
				k8sclient.FormatCPU(r.Totals.CPUUsage), k8sclient.FormatMem(r.Totals.MemUsage))
			if r.Nodes > 0 {
				fmt.Printf("allocatable over %d ready node(s): CPU %s / mem %s — requests are %d%% of CPU, %d%% of memory\n",
					r.Nodes, k8sclient.FormatCPU(r.AllocCPU), k8sclient.FormatMem(r.AllocMem),
					pct(r.Totals.CPURequest, r.AllocCPU), pct(r.Totals.MemRequest, r.AllocMem))
			}
			// Undeclared first, because it is the only one of these numbers that can
			// be compared against the container count — the three below overlap.
			fmt.Printf("undeclared: %d of %d containers (%d no CPU request, %d no memory request, %d no memory limit)\n",
				r.Totals.Undeclared, r.Totals.Containers,
				r.Totals.NoCPURequest, r.Totals.NoMemRequest, r.Totals.NoMemLimit)
			for _, a := range r.Advice {
				fmt.Printf("\n! %s\n", a)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "\nNAMESPACE\tPODS\tCPU REQ\tCPU USE\tMEM REQ\tMEM USE\tUNDECLARED\tQUOTA\tLIMITRANGES")
			for _, ns := range r.Namespaces {
				fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%d of %d\t%d\t%d\n", ns.Namespace, ns.Pods,
					k8sclient.FormatCPU(ns.CPURequest), k8sclient.FormatCPU(ns.CPUUsage),
					k8sclient.FormatMem(ns.MemRequest), k8sclient.FormatMem(ns.MemUsage),
					ns.Undeclared, ns.Containers, len(ns.Quota), ns.LimitRanges)
			}
			if err := w.Flush(); err != nil {
				return err
			}

			if len(r.Containers) == 0 {
				fmt.Println("\nnothing to flag.")
				return nil
			}
			fmt.Printf("\n%d container(s) worth a look:\n", len(r.Containers))
			for _, c := range r.Containers {
				fmt.Printf("  %s/%s [%s] %s\n", c.Namespace, c.Pod, c.Container, c.QoS)
				for _, f := range c.Findings {
					fmt.Printf("      - %s\n", f)
				}
			}
			return nil
		},
	}
	sizingCmd.Flags().StringVarP(&sizingNs, "namespace", "n", "", "namespace (rỗng = cả cluster)")
	root.AddCommand(sizingCmd)

	var evNamespace string
	eventsCmd := &cobra.Command{
		Use:   "events <kind> <name>",
		Short: "Liệt kê event của một resource (giống kubectl describe)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			events, err := k8sclient.ListEvents(context.Background(), cluster, args[0], evNamespace, args[1])
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "TYPE\tREASON\tAGE\tMESSAGE")
			for _, e := range events {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Type, e.Reason, e.Age, e.Message)
			}
			return w.Flush()
		},
	}
	eventsCmd.Flags().StringVarP(&evNamespace, "namespace", "n", "default", "namespace")
	root.AddCommand(eventsCmd)

	var scaleNs string
	var scaleN int
	scaleCmd := &cobra.Command{
		Use:   "scale <deployment>",
		Short: "Đổi số replica của deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			if err := k8sclient.ScaleDeployment(context.Background(), cluster, scaleNs, args[0], int32(scaleN)); err != nil {
				return err
			}
			fmt.Printf("scaled %s to %d\n", args[0], scaleN)
			return nil
		},
	}
	scaleCmd.Flags().StringVarP(&scaleNs, "namespace", "n", "default", "namespace")
	scaleCmd.Flags().IntVar(&scaleN, "replicas", 1, "số replica")
	root.AddCommand(scaleCmd)

	var restartNs string
	restartCmd := &cobra.Command{
		Use:   "restart <deployment>",
		Short: "Rolling restart deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			if err := k8sclient.RestartDeployment(context.Background(), cluster, restartNs, args[0], time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
			fmt.Println("restarted")
			return nil
		},
	}
	restartCmd.Flags().StringVarP(&restartNs, "namespace", "n", "default", "namespace")
	root.AddCommand(restartCmd)

	var pfNs, pfKind string
	var pfLocal, pfRemote int
	var pfHold int
	pfCmd := &cobra.Command{
		Use:   "port-forward <name>",
		Short: "Forward a local port to a Pod/Service port (holds open)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			session, ready, errCh, err := k8sclient.StartPortForward(context.Background(), cluster, pfKind, pfNs, args[0], pfLocal, pfRemote)
			if err != nil {
				return err
			}
			defer session.Close()
			select {
			case <-ready:
				fmt.Printf("forwarding localhost:%d -> %s:%d (pod %s)\n", session.LocalPort, args[0], pfRemote, session.PodName)
			case err := <-errCh:
				return fmt.Errorf("failed: %w", err)
			case <-time.After(15 * time.Second):
				return fmt.Errorf("not ready in time")
			}
			select {
			case <-time.After(time.Duration(pfHold) * time.Second):
			case err := <-errCh:
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
	pfCmd.Flags().StringVarP(&pfNs, "namespace", "n", "default", "namespace")
	pfCmd.Flags().StringVar(&pfKind, "kind", "Pod", "Pod or Service")
	pfCmd.Flags().IntVar(&pfLocal, "local", 0, "local port (0 = auto)")
	pfCmd.Flags().IntVar(&pfRemote, "remote", 80, "remote port")
	pfCmd.Flags().IntVar(&pfHold, "hold", 10, "seconds to hold the tunnel open")
	root.AddCommand(pfCmd)

	var execNs, execContainer, execShell string
	execCmd := &cobra.Command{
		Use:   "exec <pod> -- <command...>",
		Short: "Run a command in a container via the exec backend",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			command := "echo kubby-exec-ok; id"
			if len(args) > 1 {
				command = strings.Join(args[1:], " ")
			}
			done := make(chan error, 1)
			session, err := k8sclient.StartExec(context.Background(), cluster, execNs, args[0], execContainer, execShell, 80, 24,
				func(out string) { fmt.Print(out) },
				func(err error) { done <- err })
			if err != nil {
				return err
			}
			_ = session.Write(command + "\nexit\n")
			select {
			case err := <-done:
				return err
			case <-time.After(15 * time.Second):
				session.Close()
				return fmt.Errorf("exec timed out")
			}
		},
	}
	execCmd.Flags().StringVarP(&execNs, "namespace", "n", "default", "namespace")
	execCmd.Flags().StringVarP(&execContainer, "container", "c", "", "container")
	execCmd.Flags().StringVar(&execShell, "shell", "/bin/sh", "shell")
	root.AddCommand(execCmd)

	cordonCmd := &cobra.Command{
		Use:   "cordon <node>",
		Short: "Cordon (mark unschedulable) a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			if err := k8sclient.SetNodeSchedulable(context.Background(), cluster, args[0], false); err != nil {
				return err
			}
			fmt.Println("cordoned")
			return nil
		},
	}
	root.AddCommand(cordonCmd)

	uncordonCmd := &cobra.Command{
		Use:   "uncordon <node>",
		Short: "Uncordon a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			if err := k8sclient.SetNodeSchedulable(context.Background(), cluster, args[0], true); err != nil {
				return err
			}
			fmt.Println("uncordoned")
			return nil
		},
	}
	root.AddCommand(uncordonCmd)

	var rolloutNs string
	rolloutCmd := &cobra.Command{
		Use:   "rollout <deployment>",
		Short: "Show rollout history of a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			revs, err := k8sclient.RolloutHistory(context.Background(), cluster, rolloutNs, args[0])
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "REVISION\tCURRENT\tIMAGES\tAGE")
			for _, r := range revs {
				fmt.Fprintf(w, "%d\t%t\t%s\t%s\n", r.Revision, r.Current, r.Images, r.Age)
			}
			return w.Flush()
		},
	}
	rolloutCmd.Flags().StringVarP(&rolloutNs, "namespace", "n", "default", "namespace")
	root.AddCommand(rolloutCmd)

	helmSearchCmd := &cobra.Command{
		Use:   "helm-search <query>",
		Short: "Search Helm charts on Artifact Hub",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := k8sclient.SearchCharts(context.Background(), args[0])
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tREPO\tVERSION\tREPO_URL")
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Repo, r.Version, r.RepoURL)
			}
			return w.Flush()
		},
	}
	root.AddCommand(helmSearchCmd)

	var hiNs, hiRepo, hiRepoName, hiChart, hiVersion string
	helmInstallCmd := &cobra.Command{
		Use:   "helm-install <release>",
		Short: "Install a Helm chart from a repo URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			preview, err := k8sclient.HelmInstallPreview(context.Background(), cluster, hiNs, args[0], hiRepo, hiRepoName, hiChart, hiVersion, "")
			if err != nil {
				return fmt.Errorf("preview install: %w", err)
			}
			if err := k8sclient.HelmInstall(context.Background(), cluster, hiNs, args[0], hiRepo, hiRepoName, hiChart, hiVersion, "", preview.ChartDigest); err != nil {
				return err
			}
			fmt.Println("installed")
			return nil
		},
	}
	helmInstallCmd.Flags().StringVarP(&hiNs, "namespace", "n", "default", "namespace")
	helmInstallCmd.Flags().StringVar(&hiRepo, "repo", "", "chart repo URL")
	helmInstallCmd.Flags().StringVar(&hiRepoName, "repo-name", "", "configured Helm repository name (uses its saved credentials)")
	helmInstallCmd.Flags().StringVar(&hiChart, "chart", "", "chart name")
	helmInstallCmd.Flags().StringVar(&hiVersion, "version", "", "chart version")
	root.AddCommand(helmInstallCmd)

	var huNs string
	helmUninstallCmd := &cobra.Command{
		Use:   "helm-uninstall <release>",
		Short: "Uninstall a Helm release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			if err := k8sclient.HelmUninstall(context.Background(), cluster, huNs, args[0]); err != nil {
				return err
			}
			fmt.Println("uninstalled")
			return nil
		},
	}
	helmUninstallCmd.Flags().StringVarP(&huNs, "namespace", "n", "default", "namespace")
	root.AddCommand(helmUninstallCmd)

	// chart-details <repo> <chart> — Artifact Hub package detail (versions/readme/values).
	chartDetailsCmd := &cobra.Command{
		Use:   "chart-details <repo> <chart>",
		Short: "Show Artifact Hub detail for a chart",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := k8sclient.ChartDetails(context.Background(), args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Printf("name=%s repo=%s version=%s app=%s\nhome=%s\nversions=%v\nmaintainers=%v\ndefaultValues bytes=%d readme bytes=%d\n",
				d.Name, d.Repo, d.Version, d.AppVersion, d.HomeURL, d.Versions, d.Maintainers, len(d.DefaultValues), len(d.Readme))
			return nil
		},
	}
	root.AddCommand(chartDetailsCmd)

	// chart-values <repoURL> <chart> <version> — authoritative default values.yaml.
	chartValuesCmd := &cobra.Command{
		Use:   "chart-values <repoURL> <chart> <version>",
		Short: "Fetch a chart's default values.yaml from its repo",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			vals, err := k8sclient.ChartDefaultValues(args[0], "", args[1], args[2])
			if err != nil {
				return err
			}
			preview := vals
			if len(preview) > 400 {
				preview = preview[:400] + "…"
			}
			fmt.Printf("default values.yaml bytes=%d\n---\n%s\n", len(vals), preview)
			return nil
		},
	}
	root.AddCommand(chartValuesCmd)

	// helm-preview <release> — dry-run upgrade diff (proposed vs current manifest bytes).
	var hpNs string
	helmPreviewCmd := &cobra.Command{
		Use:   "helm-preview <release>",
		Short: "Dry-run upgrade preview of a release (reuses current values)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			cur, err := k8sclient.HelmGet(context.Background(), cluster, hpNs, args[0])
			if err != nil {
				return err
			}
			diff, err := k8sclient.HelmUpgradePreview(context.Background(), cluster, hpNs, args[0], cur.Values)
			if err != nil {
				return err
			}
			fmt.Printf("current manifest bytes=%d proposed manifest bytes=%d\n", len(diff.Current), len(diff.Proposed))
			return nil
		},
	}
	helmPreviewCmd.Flags().StringVarP(&hpNs, "namespace", "n", "default", "namespace")
	root.AddCommand(helmPreviewCmd)

	// helm-resources <release> — objects owned by a release + live health.
	var hrNs string
	helmResourcesCmd := &cobra.Command{
		Use:   "helm-resources <release>",
		Short: "List objects a Helm release owns, with live health",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			res, err := k8sclient.HelmReleaseResources(context.Background(), cluster, hrNs, args[0])
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KIND\tNAME\tNAMESPACE\tSTATUS\tREADY")
			for _, r := range res {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%v\n", r.Kind, r.Name, r.Namespace, r.Status, r.Ready)
			}
			return w.Flush()
		},
	}
	helmResourcesCmd.Flags().StringVarP(&hrNs, "namespace", "n", "default", "namespace")
	root.AddCommand(helmResourcesCmd)

	// repo subcommands (add/list/browse) share the user's Helm repo config.
	repoAddCmd := &cobra.Command{
		Use:   "repo-add <name> <url>",
		Short: "Add a Helm chart repository",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := k8sclient.AddHelmRepo(args[0], args[1], "", ""); err != nil {
				return err
			}
			fmt.Println("added")
			return nil
		},
	}
	root.AddCommand(repoAddCmd)

	repoListCmd := &cobra.Command{
		Use:   "repo-list",
		Short: "List configured Helm repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, err := k8sclient.ListHelmRepos()
			if err != nil {
				return err
			}
			for _, r := range repos {
				fmt.Printf("%s\t%s\n", r.Name, r.URL)
			}
			return nil
		},
	}
	root.AddCommand(repoListCmd)

	repoBrowseCmd := &cobra.Command{
		Use:   "repo-browse <name>",
		Short: "List charts in a configured repo's cached index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			charts, err := k8sclient.BrowseHelmRepo(args[0])
			if err != nil {
				return err
			}
			fmt.Printf("%d charts\n", len(charts))
			for i, c := range charts {
				if i >= 10 {
					fmt.Println("…")
					break
				}
				fmt.Printf("%s\t%s\t%s\n", c.Name, c.Version, c.AppVersion)
			}
			return nil
		},
	}
	root.AddCommand(repoBrowseCmd)

	// node-pods <node> — pods scheduled on a node.
	nodePodsCmd := &cobra.Command{
		Use:   "node-pods <node>",
		Short: "List pods scheduled on a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			pods, err := k8sclient.PodsOnNode(context.Background(), cluster, args[0])
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAMESPACE\tNAME\tSTATUS\tREADY")
			for _, p := range pods {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Namespace, p.Name, p.Status, p.Ready)
			}
			return w.Flush()
		},
	}
	root.AddCommand(nodePodsCmd)

	// ns-summary <ns> — per-kind counts in a namespace.
	nsSummaryCmd := &cobra.Command{
		Use:   "ns-summary <namespace>",
		Short: "Summarize resource counts in a namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			counts, err := k8sclient.NamespaceSummary(context.Background(), cluster, args[0])
			if err != nil {
				return err
			}
			for _, c := range counts {
				fmt.Printf("%-14s %d (errors %d)\n", c.Kind, c.Count, c.Errors)
			}
			return nil
		},
	}
	root.AddCommand(nsSummaryCmd)

	// search <query> — global resource name search.
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Find resources by name across kinds/namespaces",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			hits, err := k8sclient.SearchResources(context.Background(), cluster, args[0])
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "%d hits\n", len(hits))
			fmt.Fprintln(w, "KIND\tNAMESPACE\tNAME")
			for _, h := range hits {
				fmt.Fprintf(w, "%s\t%s\t%s\n", h.Kind, h.Namespace, h.Name)
			}
			return w.Flush()
		},
	}
	root.AddCommand(searchCmd)

	// netflows — Ingress → Service → Pod topology.
	var netflowsNs string
	netflowsCmd := &cobra.Command{
		Use:   "netflows",
		Short: "Show entry-point → Service → Pod topology (Ingress and Istio Gateway)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			flows, err := k8sclient.NetworkTopology(context.Background(), cluster, netflowsNs)
			if err != nil {
				return err
			}
			gateways := 0
			for _, ing := range flows.Ingresses {
				if ing.Kind == "Gateway" {
					gateways++
				}
			}
			fmt.Printf("%d entry point(s) (%d Ingress, %d Istio Gateway), %d internal service(s), %d routed service(s), %d endpoint pod(s), %d warning(s)\n",
				len(flows.Ingresses), len(flows.Ingresses)-gateways, gateways,
				len(flows.Services), flows.RoutedCount, flows.EndpointCount, flows.BrokenCount)
			printSvc := func(indent string, svc k8sclient.FlowService) {
				fmt.Printf("%s→ Service %s/%s [%s %s] %v\n", indent, svc.Namespace, svc.Name,
					svc.Type, svc.ClusterIP, svc.Ports)
				if svc.Via != "" {
					fmt.Printf("%s    via %s/%s\n", indent, svc.ViaNamespace, svc.Via)
				}
				for _, r := range svc.Routes {
					fmt.Printf("%s    route %s\n", indent, r)
				}
				if svc.Warning != "" {
					fmt.Printf("%s    ! %s\n", indent, svc.Warning)
				}
				for _, pod := range svc.Pods {
					fmt.Printf("%s    → Pod %s [%s %s] on %s\n", indent, pod.Name, pod.Status, pod.Ready, pod.Node)
				}
			}
			for _, ing := range flows.Ingresses {
				fmt.Printf("%s %s/%s class=%q hosts=%v ports=%v tls=%v addr=%q\n",
					ing.Kind, ing.Namespace, ing.Name, ing.Class, ing.Hosts, ing.Ports, ing.TLS, ing.Address)
				if ing.Warning != "" {
					fmt.Printf("  ! %s\n", ing.Warning)
				}
				for _, svc := range ing.Services {
					printSvc("  ", svc)
				}
			}
			for _, svc := range flows.Services {
				printSvc("", svc)
			}
			return nil
		},
	}
	netflowsCmd.Flags().StringVarP(&netflowsNs, "namespace", "n", "", "namespace (default: all)")
	root.AddCommand(netflowsCmd)

	// counts — the one-call sidebar tallies (timed, to compare against the old per-kind calls).
	var countsNs string
	var countsCluster bool
	countsCmd := &cobra.Command{
		Use:   "counts",
		Short: "Print the sidebar counts for a namespace, with the elapsed time",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			start := time.Now()
			counts, err := k8sclient.SidebarCounts(context.Background(), cluster, countsNs, countsCluster)
			if err != nil {
				return err
			}
			fmt.Printf("%d tallies in %s (namespace=%q, cluster-scoped=%v)\n",
				len(counts), time.Since(start).Round(time.Millisecond), countsNs, countsCluster)
			for _, c := range counts {
				errs := ""
				if c.Errors > 0 {
					errs = fmt.Sprintf("  ! %d with errors", c.Errors)
				}
				fmt.Printf("  %-22s %4d%s\n", c.View, c.Count, errs)
			}
			return nil
		},
	}
	countsCmd.Flags().StringVarP(&countsNs, "namespace", "n", "", "namespace (default: all)")
	countsCmd.Flags().BoolVar(&countsCluster, "cluster", true, "include cluster-scoped kinds")
	root.AddCommand(countsCmd)

	// custom-kinds — the CRD-defined kinds that become their own sidebar sections.
	customKindsCmd := &cobra.Command{
		Use:   "custom-kinds",
		Short: "List the CRD-defined kinds the sidebar shows as sections",
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			start := time.Now()
			out, err := k8sclient.CustomKinds(context.Background(), cluster)
			if err != nil {
				return err
			}
			fmt.Printf("%d custom kind(s) in %s (%d shown, %d over the cap)\n",
				out.Total, time.Since(start).Round(time.Millisecond), len(out.Kinds), out.Overflow)
			for _, k := range out.Kinds {
				scope := "cluster"
				if k.Namespaced {
					scope = "namespaced"
				}
				fmt.Printf("  %-24s %-34s %s\n", k.Title, k.RefKind, scope)
			}
			return nil
		},
	}
	root.AddCommand(customKindsCmd)

	// list-custom <Kind.group> — the generic list behind a custom-resource section.
	var listCustomNs string
	listCustomCmd := &cobra.Command{
		Use:   "list-custom <Kind.group>",
		Short: "List the objects of a custom kind",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			items, err := k8sclient.ListCustom(context.Background(), cluster, args[0], listCustomNs)
			if err != nil {
				return err
			}
			fmt.Printf("%d object(s)\n", len(items))
			for _, it := range items {
				fmt.Printf("  %-20s %-40s %-6s %s\n", it.Namespace, it.Name, it.Age, it.Status)
			}
			return nil
		},
	}
	listCustomCmd.Flags().StringVarP(&listCustomNs, "namespace", "n", "", "namespace (default: all)")
	root.AddCommand(listCustomCmd)

	// diag <kind> <name> — the AI diagnostic context (events+logs+yaml) for a resource.
	var diagNs string
	diagCmd := &cobra.Command{
		Use:   "diag <kind> <name>",
		Short: "Print the AI diagnostic context for a resource (no AI call)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cluster, err := k8sclient.New(kubeconfigPath, kubeContext)
			if err != nil {
				return err
			}
			out, err := k8sclient.DiagnosticContext(context.Background(), cluster, args[0], diagNs, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("# context: %d events, %d container log(s) (%d lines), yaml=%v, %d chars\n\n",
				out.Events, out.LogContainers, out.LogLines, out.HasYAML, out.Chars)
			fmt.Println(out.Text)
			return nil
		},
	}
	diagCmd.Flags().StringVarP(&diagNs, "namespace", "n", "default", "namespace")
	root.AddCommand(diagCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "lỗi:", err)
		os.Exit(1)
	}
}

// unifiedDiff renders a +/- line diff for the `diff` command, showing only
// changed lines plus a little context so a 400-line object does not print whole.
// LCS is O(n*m), which is fine for single manifests.
//
// The GUI has its own renderer (lineDiff in main.js); this exists so the preview
// is checkable without opening the app.
func unifiedDiff(before, after string) []string {
	const context = 3
	a, b := splitLines(before), splitLines(after)
	n, m := len(a), len(b)

	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	type change struct {
		sign byte
		text string
	}
	all := []change{}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			all = append(all, change{' ', a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			all = append(all, change{'-', a[i]})
			i++
		default:
			all = append(all, change{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		all = append(all, change{'-', a[i]})
	}
	for ; j < m; j++ {
		all = append(all, change{'+', b[j]})
	}

	// Keep a changed line and `context` lines either side of it.
	keep := make([]bool, len(all))
	for k, c := range all {
		if c.sign == ' ' {
			continue
		}
		for d := k - context; d <= k+context; d++ {
			if d >= 0 && d < len(all) {
				keep[d] = true
			}
		}
	}

	out := []string{}
	skipping := false
	for k, c := range all {
		if !keep[k] {
			if !skipping {
				out = append(out, "    …")
				skipping = true
			}
			continue
		}
		skipping = false
		out = append(out, fmt.Sprintf("  %c %s", c.sign, c.text))
	}
	return out
}

// pct is integer percent, guarding the empty-cluster divide-by-zero.
func pct(part, whole int64) int64 {
	if whole <= 0 {
		return 0
	}
	return part * 100 / whole
}

func splitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
