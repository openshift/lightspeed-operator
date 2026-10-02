package utils

import (
	"context"
	"errors"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type directVersionClient struct {
	client.Client
	reader client.Reader
}

func (c directVersionClient) GetAPIReader() client.Reader { return c.reader }

type unreadableVersionReader struct{ client.Reader }

func (unreadableVersionReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return errors.New("unavailable")
}

func TestAgenticEnabledTracksLiveClusterVersion(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := configv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cv := &configv1.ClusterVersion{ObjectMeta: metav1.ObjectMeta{Name: "version"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cv).WithObjects(cv).Build()
	check := func(want bool) {
		t.Helper()
		if got := AgenticEnabled(c, ctx); got != want {
			t.Fatalf("AgenticEnabled = %v, want %v", got, want)
		}
	}
	check(false) // unknown on fresh install
	for _, step := range []struct {
		version   string
		completed string
		enabled   bool
	}{
		{"4.23.0", "4.23.0", false},
		{"5.0.0", "4.23.0", false}, // upgrade started, not completed
		{"5.0.0", "5.0.0", true},
		{"4.22.0", "5.0.0", false}, // downgrade started
		{"4.22.0", "4.22.0", false},
		{"5.1.0", "5.1.0", true},
		{"5.invalid", "5.invalid", false},
		{"", "", false},
	} {
		current := &configv1.ClusterVersion{}
		if err := c.Get(ctx, client.ObjectKey{Name: "version"}, current); err != nil {
			t.Fatal(err)
		}
		current.Status.Desired.Version = step.version
		current.Status.History = []configv1.UpdateHistory{{Version: step.completed, State: configv1.CompletedUpdate}}
		if err := c.Status().Update(ctx, current); err != nil {
			t.Fatal(err)
		}
		check(step.enabled)
	}
	// Even an apparent 5.0 desired version without history must fail closed.
	current := &configv1.ClusterVersion{}
	if err := c.Get(ctx, client.ObjectKey{Name: "version"}, current); err != nil {
		t.Fatal(err)
	}
	current.Status.Desired.Version = "5.0.0"
	current.Status.History = nil
	if err := c.Status().Update(ctx, current); err != nil {
		t.Fatal(err)
	}
	check(false)
	current.Status.History = []configv1.UpdateHistory{{Version: "5.0.0", State: configv1.CompletedUpdate}}
	if err := c.Status().Update(ctx, current); err != nil {
		t.Fatal(err)
	}
	// The cached client still reports 5.0, but an unreadable direct API
	// response must not authorize agentic resources.
	check(true)
	older := &configv1.ClusterVersion{ObjectMeta: metav1.ObjectMeta{Name: "version"}}
	older.Status.Desired.Version = "4.22.0"
	older.Status.History = []configv1.UpdateHistory{{Version: "4.22.0", State: configv1.CompletedUpdate}}
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(older).Build()
	if AgenticEnabled(directVersionClient{Client: c, reader: live}, ctx) {
		t.Fatal("cached 5.0 enabled agentic after the live version regressed")
	}
	if AgenticEnabled(directVersionClient{Client: c, reader: unreadableVersionReader{}}, ctx) {
		t.Fatal("stale cache enabled agentic after a direct read failure")
	}
	if version, err := ReadAgenticVersion(directVersionClient{Client: c, reader: unreadableVersionReader{}}, ctx); err == nil || version.Enabled {
		t.Fatalf("transient read must fail closed and trigger retry: version=%+v, err=%v", version, err)
	}
	if !AgenticEnabled(directVersionClient{Client: c, reader: unreadableVersionReader{}}, WithAgenticVersion(ctx, AgenticVersion{Enabled: true, Major: "5", Minor: "0"})) {
		t.Fatal("reconcile lost its completed-version snapshot during a transient read failure")
	}
	if AgenticEnabled(directVersionClient{Client: c}, ctx) {
		t.Fatal("missing direct reader enabled agentic")
	}
	if err := c.Delete(ctx, cv); err != nil {
		t.Fatal(err)
	}
	check(false)
	if version, err := ReadAgenticVersion(c, ctx); err == nil || version.Enabled {
		t.Fatalf("missing ClusterVersion should be retried: version=%+v, err=%v", version, err)
	}
}
