package k8sclient

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metadatafake "k8s.io/client-go/metadata/fake"
)

func TestListHelmReleasesKeepsOnlyLatestLiveRevision(t *testing.T) {
	now := time.Now()
	terminating := metav1.NewTime(now.Add(time.Minute))
	client := newHelmMetadataClient(
		helmSecret("apps", "sh.helm.release.v1.api.v1", "api", "1", "superseded", now),
		helmSecret("apps", "sh.helm.release.v1.api.v2", "api", "2", "deployed", now.Add(time.Second)),
		helmSecret("ops", "sh.helm.release.v1.agent.v3", "agent", "3", "pending-upgrade", now.Add(2*time.Second)),
		func() *metav1.PartialObjectMetadata {
			item := helmSecret("ops", "sh.helm.release.v1.old.v9", "old", "9", "failed", now.Add(3*time.Second))
			item.DeletionTimestamp = &terminating
			return item
		}(),
	)

	releases, err := ListHelmReleases(context.Background(), client, metav1.NamespaceAll)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Fatalf("got %d release rows, want 2 current releases: %#v", len(releases), releases)
	}
	if releases[0].Name != "api" || releases[0].Revision != "2" || releases[0].Status != "deployed" {
		t.Fatalf("first release = %#v, want latest api revision", releases[0])
	}
	if releases[1].Name != "agent" || !releases[1].IsPending || releases[1].IsError {
		t.Fatalf("pending release classification = %#v", releases[1])
	}
}

func newHelmMetadataClient(objects ...runtime.Object) *metadatafake.FakeMetadataClient {
	scheme := metadatafake.NewTestScheme()
	metav1.AddMetaToScheme(scheme)
	return metadatafake.NewSimpleMetadataClient(scheme, objects...)
}

func helmSecret(namespace, secretName, releaseName, revision, status string, created time.Time) *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace, Name: secretName, CreationTimestamp: metav1.NewTime(created),
			Labels: map[string]string{"owner": "helm", "name": releaseName, "version": revision, "status": status},
		},
	}
}
