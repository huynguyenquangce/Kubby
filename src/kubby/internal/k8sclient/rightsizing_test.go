package k8sclient

import (
	"strings"
	"testing"
)

// "Not declared" and "declared as zero" are different facts, and every finding in
// this file depends on telling them apart. A regression that turns unset into 0
// would make the report claim every container has a limit.
func TestUnsetIsNeverRenderedAsZero(t *testing.T) {
	if got := FormatCPU(unset); got != "—" {
		t.Errorf("FormatCPU(unset) = %q; want a dash, not a number", got)
	}
	if got := FormatMem(unset); got != "—" {
		t.Errorf("FormatMem(unset) = %q; want a dash, not a number", got)
	}
	if got := FormatCPU(0); got != "0m" {
		t.Errorf("FormatCPU(0) = %q; a declared zero must still read as zero", got)
	}
}

func TestFormatUnits(t *testing.T) {
	cases := []struct {
		milli int64
		want  string
	}{{250, "250m"}, {999, "999m"}, {1000, "1.00"}, {1150, "1.15"}}
	for _, c := range cases {
		if got := FormatCPU(c.milli); got != c.want {
			t.Errorf("FormatCPU(%d) = %q; want %q", c.milli, got, c.want)
		}
	}
	const mi = 1 << 20
	memCases := []struct {
		bytes int64
		want  string
	}{{512 * mi, "512Mi"}, {1024 * mi, "1.0Gi"}, {1536 * mi, "1.5Gi"}}
	for _, c := range memCases {
		if got := FormatMem(c.bytes); got != c.want {
			t.Errorf("FormatMem(%d) = %q; want %q", c.bytes, got, c.want)
		}
	}
}

func TestPercentOfRefusesToInvent(t *testing.T) {
	if got := percentOf(unset, 100); got != unset {
		t.Errorf("percentOf(unknown usage) = %d; want unset", got)
	}
	if got := percentOf(50, unset); got != unset {
		t.Errorf("percentOf(against an undeclared request) = %d; want unset", got)
	}
	// A container requesting 0 with usage above it is not "infinitely over" —
	// dividing would panic, and reporting a number would be a lie.
	if got := percentOf(50, 0); got != unset {
		t.Errorf("percentOf(x, 0) = %d; want unset", got)
	}
	if got := percentOf(25, 200); got != 12 {
		t.Errorf("percentOf(25, 200) = %d; want 12", got)
	}
}

func TestFindingsForAnUndeclaredContainer(t *testing.T) {
	cs := ContainerSizing{
		CPURequest: unset, CPULimit: unset, MemRequest: unset, MemLimit: unset,
		CPUUsage: unset, MemUsage: unset, CPUPct: unset, MemPct: unset, MemOfLimit: unset,
	}
	got := findings(&cs)
	for _, want := range []string{"no memory limit", "no memory request", "no CPU request"} {
		if !containsSubstring(got, want) {
			t.Errorf("findings missing %q; got %v", want, got)
		}
	}
}

// An OOMKill is the most serious thing this report can find, and it must lead —
// the UI shows the first finding in a narrow column.
func TestOOMKillLeadsTheFindings(t *testing.T) {
	cs := ContainerSizing{
		OOMKilled: true,
		MemLimit:  256 << 20, MemUsage: 250 << 20, MemOfLimit: 97,
		CPURequest: 100, CPUUsage: 10, CPUPct: 10,
		MemRequest: 128 << 20, MemPct: 195,
	}
	got := findings(&cs)
	if len(got) == 0 || !strings.Contains(got[0], "OOMKilled") {
		t.Fatalf("first finding = %v; want the OOMKill first", got)
	}
	if severity(&cs) <= severity(&ContainerSizing{MemLimit: unset}) {
		t.Error("an OOMKilled container must outrank one that merely has no limit")
	}
}

// Over-provisioning is only worth reporting when the reservation is big enough to
// matter. Flagging a 10m request as idle would bury the findings that count.
func TestTinyRequestsAreNotCalledOverProvisioned(t *testing.T) {
	small := ContainerSizing{
		CPURequest: 10, CPUUsage: 1, CPUPct: 10,
		MemRequest: 8 << 20, MemUsage: 1 << 20, MemPct: 12,
		MemLimit: 64 << 20, MemOfLimit: 2,
	}
	if got := findings(&small); containsSubstring(got, "of its") {
		t.Errorf("a 10m request should not be flagged as idle; got %v", got)
	}

	big := ContainerSizing{
		CPURequest: 2000, CPUUsage: 20, CPUPct: 1,
		MemRequest: unset, MemLimit: 512 << 20, MemOfLimit: 3,
		MemUsage: unset, MemPct: unset,
	}
	if got := findings(&big); !containsSubstring(got, "of its 2.00 CPU request") {
		t.Errorf("a 2-core request used at 1%% should be flagged; got %v", got)
	}
}

func TestSeverityOrdering(t *testing.T) {
	oom := ContainerSizing{OOMKilled: true}
	nearLimit := ContainerSizing{MemOfLimit: 95}
	noLimit := ContainerSizing{MemLimit: unset}
	noRequest := ContainerSizing{MemRequest: unset}
	fine := ContainerSizing{MemLimit: 1, MemRequest: 1, CPURequest: 1}

	ranks := []int{severity(&oom), severity(&nearLimit), severity(&noLimit), severity(&noRequest), severity(&fine)}
	for i := 1; i < len(ranks); i++ {
		if ranks[i-1] <= ranks[i] {
			t.Fatalf("severity is not strictly descending: %v", ranks)
		}
	}
}

// The quota check compares two strings that came from the same ResourceQuota, so
// it must handle the unit suffixes Kubernetes actually writes.
func TestQuotaNearlyFull(t *testing.T) {
	cases := []struct {
		hard, used string
		wantNear   bool
		wantPct    int64
	}{
		{"10", "9", true, 90},
		{"10", "4", false, 40},
		{"4Gi", "3.9Gi", true, 97},
		{"2", "1500m", false, 75},
		{"0", "0", false, 0},        // a zero quota cannot be a percentage
		{"nonsense", "1", false, 0}, // unparseable must not be reported as full
	}
	for _, c := range cases {
		near, pct := quotaNearlyFull(QuotaLine{Hard: c.hard, Used: c.used})
		if near != c.wantNear {
			t.Errorf("quotaNearlyFull(hard=%s used=%s) near = %v; want %v", c.hard, c.used, near, c.wantNear)
		}
		if c.wantNear && pct != c.wantPct {
			t.Errorf("quotaNearlyFull(hard=%s used=%s) pct = %d; want %d", c.hard, c.used, pct, c.wantPct)
		}
	}
}

// "1 of 10 containers declare" reads as a bug in the tool, so the aggregate lines
// have to agree with themselves grammatically.
func TestAdviceUsesTheRightVerbForOne(t *testing.T) {
	r := &SizingReport{
		Totals:     NamespaceSizing{Containers: 10, NoMemLimit: 1, NoCPURequest: 1},
		Namespaces: []NamespaceSizing{{Namespace: "app", NoMemLimit: 1, LimitRanges: 0}},
	}
	for _, line := range advice(r) {
		if strings.Contains(line, "1 of 10 containers declare no") {
			t.Errorf("plural verb used for a single container: %q", line)
		}
	}
	if got := advice(r); !containsSubstring(got, "1 of 10 containers declares no memory limit") {
		t.Errorf("expected the singular form; got %v", got)
	}
}

func TestAdviceIsSilentWhenThereIsNothingToSay(t *testing.T) {
	empty := &SizingReport{Totals: NamespaceSizing{Containers: 0}}
	if got := advice(empty); len(got) != 0 {
		t.Errorf("advice on an empty scope = %v; want nothing", got)
	}
	clean := &SizingReport{
		Totals:           NamespaceSizing{Containers: 3, CPURequest: 300, CPUUsage: 280},
		AllocCPU:         4000,
		MetricsAvailable: true,
	}
	if got := advice(clean); len(got) != 0 {
		t.Errorf("advice on a well-declared, well-used scope = %v; want nothing", got)
	}
}

func containsSubstring(lines []string, want string) bool {
	for _, l := range lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}
