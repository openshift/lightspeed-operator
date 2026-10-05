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

func TestAgenticGateTracksCompletedClusterVersion(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := configv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cv := &configv1.ClusterVersion{ObjectMeta: metav1.ObjectMeta{Name: "version"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cv).WithObjects(cv).Build()
	check := func(want AgenticGateState) {
		t.Helper()
		if got := AgenticGate(c, ctx); got != want {
			t.Fatalf("AgenticGate = %q, want %q", got, want)
		}
	}
	check(AgenticGateUnknown)

	for _, step := range []struct {
		version   string
		completed string
		state     configv1.UpdateState
		want      AgenticGateState
	}{
		{"4.23.0", "4.23.0", configv1.CompletedUpdate, AgenticGateDisabled},
		{"5.0.0", "4.23.0", configv1.CompletedUpdate, AgenticGateUnknown}, // upgrade started, not completed
		{"5.0.0", "5.0.0", configv1.CompletedUpdate, AgenticGateEnabled},
		{"4.22.0", "5.0.0", configv1.CompletedUpdate, AgenticGateUnknown}, // desired and completed versions differ
		{"5.invalid", "5.invalid", configv1.CompletedUpdate, AgenticGateUnknown},
		{"", "", configv1.CompletedUpdate, AgenticGateUnknown},
	} {
		current := &configv1.ClusterVersion{}
		if err := c.Get(ctx, client.ObjectKey{Name: "version"}, current); err != nil {
			t.Fatal(err)
		}
		current.Status.Desired.Version = step.version
		current.Status.History = []configv1.UpdateHistory{{Version: step.completed, State: step.state}}
		if err := c.Status().Update(ctx, current); err != nil {
			t.Fatal(err)
		}
		if got := AgenticGate(c, ctx); got != step.want {
			t.Fatalf("AgenticGate for desired %q/completed %q = %q, want %q", step.version, step.completed, got, step.want)
		}
	}

	current := &configv1.ClusterVersion{}
	if err := c.Get(ctx, client.ObjectKey{Name: "version"}, current); err != nil {
		t.Fatal(err)
	}
	current.Status.Desired.Version = "5.0.0"
	current.Status.History = nil
	if err := c.Status().Update(ctx, current); err != nil {
		t.Fatal(err)
	}
	check(AgenticGateUnknown)

	older := &configv1.ClusterVersion{ObjectMeta: metav1.ObjectMeta{Name: "version"}}
	older.Status.Desired.Version = "4.22.0"
	older.Status.History = []configv1.UpdateHistory{{Version: "4.22.0", State: configv1.CompletedUpdate}}
	live := fake.NewClientBuilder().WithScheme(scheme).WithObjects(older).Build()
	if got := AgenticGate(directVersionClient{Client: c, reader: live}, ctx); got != AgenticGateDisabled {
		t.Fatalf("direct Version CR read returned %q, want Disabled", got)
	}

	unreadable := directVersionClient{Client: c, reader: unreadableVersionReader{}}
	if got := AgenticGate(unreadable, ctx); got != AgenticGateUnknown {
		t.Fatalf("unreadable Version CR returned %q, want Unknown", got)
	}
	if version, err := ReadAgenticVersion(unreadable, ctx); err == nil || version.State != AgenticGateUnknown {
		t.Fatalf("transient read must return Unknown and an error: version=%+v, err=%v", version, err)
	}
	if got := AgenticGate(unreadable, WithAgenticVersion(ctx, AgenticVersion{State: AgenticGateEnabled, Major: "5", Minor: "0"})); got != AgenticGateEnabled {
		t.Fatalf("reconcile did not retain its version snapshot: got %q", got)
	}
	if got := AgenticGate(directVersionClient{Client: c}, ctx); got != AgenticGateUnknown {
		t.Fatalf("missing direct reader returned %q, want Unknown", got)
	}
	if err := c.Delete(ctx, cv); err != nil {
		t.Fatal(err)
	}
	check(AgenticGateUnknown)
	if version, err := ReadAgenticVersion(c, ctx); err == nil || version.State != AgenticGateUnknown {
		t.Fatalf("missing ClusterVersion must return Unknown and an error: version=%+v, err=%v", version, err)
	}
}
