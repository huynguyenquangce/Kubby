package k8sclient

import (
	"context"
	"fmt"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
)

func TestSearchResourcesReusesOneMetadataIndexAcrossQueries(t *testing.T) {
	meta := searchMetadataClient(
		partialMetadata("apps/v1", "Deployment", "team-a", "api-server"),
		partialMetadata("apps/v1", "Deployment", "team-b", "worker"),
	)
	cluster := &Cluster{Meta: meta}

	first, err := SearchResources(context.Background(), cluster, "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Name != "api-server" {
		t.Fatalf("first search = %#v", first)
	}
	listsAfterFirst := metadataListActions(meta)
	if listsAfterFirst != len(searchKinds) {
		t.Fatalf("cold index list calls = %d, want %d", listsAfterFirst, len(searchKinds))
	}

	second, err := SearchResources(context.Background(), cluster, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].Name != "worker" {
		t.Fatalf("warm search = %#v", second)
	}
	if got := metadataListActions(meta); got != listsAfterFirst {
		t.Fatalf("warm search issued %d total lists, want unchanged %d", got, listsAfterFirst)
	}
}

func TestConcurrentSearchesCoalesceColdIndexRefresh(t *testing.T) {
	meta := searchMetadataClient(partialMetadata("apps/v1", "Deployment", "default", "api"))
	cluster := &Cluster{Meta: meta}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, query := range []string{"api", "missing"} {
		query := query
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := SearchResources(context.Background(), cluster, query); err != nil {
				t.Errorf("SearchResources(%q): %v", query, err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := metadataListActions(meta); got != len(searchKinds) {
		t.Fatalf("concurrent cold searches issued %d lists, want one %d-kind fan-out", got, len(searchKinds))
	}
}

func BenchmarkSearchResourcesWarmIndex10kObjects(b *testing.B) {
	objects := make([]runtime.Object, 10_000)
	for i := range objects {
		objects[i] = partialMetadata("apps/v1", "Deployment", "load", fmt.Sprintf("deployment-%05d", i))
	}
	meta := searchMetadataClient(objects...)
	cluster := &Cluster{Meta: meta}
	if _, err := SearchResources(context.Background(), cluster, "deployment-09999"); err != nil {
		b.Fatal(err)
	}
	lists := metadataListActions(meta)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := SearchResources(context.Background(), cluster, "deployment-09999"); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if got := metadataListActions(meta); got != lists {
		b.Fatalf("warm benchmark issued %d lists after setup, want 0", got-lists)
	}
	b.ReportMetric(float64(lists), "cold-lists")
}

func searchMetadataClient(objects ...runtime.Object) *metadatafake.FakeMetadataClient {
	scheme := metadatafake.NewTestScheme()
	metav1.AddMetaToScheme(scheme)
	return metadatafake.NewSimpleMetadataClient(scheme, objects...)
}

func metadataListActions(client *metadatafake.FakeMetadataClient) int {
	count := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "list" {
			count++
		}
	}
	return count
}
