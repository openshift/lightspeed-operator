package appserver

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestClientCAsAreIndependentOfConsoleVersion(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{corev1.AddToScheme, configv1.AddToScheme, olsv1alpha1.AddToScheme} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	cr := utils.GetDefaultOLSConfigCR()
	introspection := true
	cr.Spec.OLSConfig.IntrospectionEnabled = &introspection
	cr.Spec.OLSConfig.ByokRAGOnly = false
	cr.UID = "config-uid"
	cv := &configv1.ClusterVersion{ObjectMeta: metav1.ObjectMeta{Name: "version"}}
	ns := utils.OLSNamespaceDefault
	source := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OLSCAConfigMap, Namespace: ns}, Data: map[string]string{utils.AppOtelCollectorCACertFile: "test-ca"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(cv).WithObjects(cr, cv, source).Build()
	r := utils.NewTestReconciler(c, logr.Discard(), scheme, ns)
	check := func(name string, exists bool) {
		t.Helper()
		err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &corev1.Secret{})
		if (err == nil) != exists {
			t.Fatalf("secret %s: exists=%v, want %v (error: %v)", name, err == nil, exists, err)
		}
	}
	setVersion := func(version string, state configv1.UpdateState) {
		t.Helper()
		obj := &configv1.ClusterVersion{}
		if err := c.Get(ctx, client.ObjectKey{Name: "version"}, obj); err != nil {
			t.Fatal(err)
		}
		obj.Status.Desired.Version = version
		obj.Status.History = []configv1.UpdateHistory{{Version: version, State: state}}
		if err := c.Status().Update(ctx, obj); err != nil {
			t.Fatal(err)
		}
		if err := RefreshClientCASecrets(r, ctx, cr); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		version string
		state   configv1.UpdateState
	}{
		{"4.23.0", configv1.CompletedUpdate},
		{"5.0.0", configv1.PartialUpdate},
		{"5.0.0", configv1.CompletedUpdate},
	} {
		setVersion(tc.version, tc.state)
		for _, name := range []string{utils.AppOtelCASecretName, utils.AppMCPCASecretName, utils.AppRHOKPCASecretName} {
			check(name, true)
		}
		for _, name := range []string{utils.AgenticOtelCASecretName, utils.AgenticMCPCASecretName, utils.AgenticRHOKPCASecretName} {
			check(name, true)
		}
	}

	// Agentic CA Secrets must rotate even while the console version is unknown.
	source.Data[utils.AppOtelCollectorCACertFile] = "rotated-ca"
	if err := c.Update(ctx, source); err != nil {
		t.Fatal(err)
	}
	setVersion("5.1.0", configv1.PartialUpdate)
	agenticKeys := map[string]string{
		utils.AgenticOtelCASecretName:  utils.AgenticOtelCASecretDataKey,
		utils.AgenticMCPCASecretName:   utils.AgenticMCPCASecretDataKey,
		utils.AgenticRHOKPCASecretName: utils.AgenticRHOKPCASecretDataKey,
	}
	for name, key := range agenticKeys {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, secret); err != nil {
			t.Fatal(err)
		}
		if got := string(secret.Data[key]); got != "rotated-ca" {
			t.Fatalf("unknown console version blocked %s CA rotation: %q", name, got)
		}
	}
	for _, cfg := range clientCASecrets[:3] {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{Name: cfg.SecretName, Namespace: ns}, secret); err != nil {
			t.Fatal(err)
		}
		if got := string(secret.Data[cfg.DataKey]); got != "rotated-ca" {
			t.Fatalf("classic CA Secret %s was not reconciled: got %q", cfg.SecretName, got)
		}
	}

	setVersion("5.1.0", configv1.CompletedUpdate)
	for name, key := range agenticKeys {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, secret); err != nil {
			t.Fatal(err)
		}
		if got := string(secret.Data[key]); got != "rotated-ca" {
			t.Fatalf("CA Secret %s was not reconciled: got %q", name, got)
		}
	}
	setVersion("4.23.0", configv1.CompletedUpdate)
	for name, key := range agenticKeys {
		secret := &corev1.Secret{}
		if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, secret); err != nil {
			t.Fatalf("console version removed Agentic CA Secret %s: %v", name, err)
		}
		if got := string(secret.Data[key]); got != "rotated-ca" {
			t.Fatalf("console version changed %s CA data unexpectedly: %q", name, got)
		}
	}
}
