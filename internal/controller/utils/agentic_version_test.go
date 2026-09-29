package utils

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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
		version string
		enabled bool
	}{
		{"4.23.0", false}, {"5.0.0", true}, {"4.22.0", false}, {"5.1.0", true},
		{"5.invalid", false}, {"", false},
	} {
		current := &configv1.ClusterVersion{}
		if err := c.Get(ctx, client.ObjectKey{Name: "version"}, current); err != nil {
			t.Fatal(err)
		}
		current.Status.Desired.Version = step.version
		if err := c.Status().Update(ctx, current); err != nil {
			t.Fatal(err)
		}
		check(step.enabled)
	}
	if err := c.Delete(ctx, cv); err != nil {
		t.Fatal(err)
	}
	check(false)
}
