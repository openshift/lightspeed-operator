package ocpmcp

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/go-logr/logr"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNetObservConfiguration(t *testing.T) {
	cases := []struct {
		name           string
		known, present bool
		prior          string
		want           bool
	}{
		{name: "confirmed presence", known: true, present: true, want: true},
		{name: "confirmed absence", known: true, prior: `toolsets = ["core", "netobserv"]`},
		{name: "unknown without prior configuration"},
		{name: "restart retains enabled", prior: `toolsets = ["core", "netobserv"]`, want: true},
		{name: "restart retains disabled", prior: `toolsets = ["core"]`},
		{name: "malformed prior configuration is repaired to base toolsets", prior: `toolsets = [`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := runtime.NewScheme()
			if err := corev1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}
			if err := networkingv1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}
			if err := olsv1alpha1.AddToScheme(s); err != nil {
				t.Fatal(err)
			}
			cr := utils.GetDefaultOLSConfigCR()
			objects := []client.Object{cr}
			if tc.prior != "" {
				objects = append(objects, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}, Data: map[string]string{utils.OpenShiftMCPServerConfigFilename: tc.prior}})
			}
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build()
			r := utils.NewTestReconciler(c, logr.Discard(), s, utils.OLSNamespaceDefault)
			var state NetObservState
			ctx := state.WithPresence(context.Background(), tc.present, tc.known)
			if err := ReconcileResources(r, ctx, cr); err != nil {
				t.Fatal(err)
			}
			cm := &corev1.ConfigMap{}
			key := client.ObjectKey{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: utils.OLSNamespaceDefault}
			if err := c.Get(ctx, key, cm); err != nil {
				t.Fatal(err)
			}
			var config struct {
				Toolsets []string `toml:"toolsets"`
			}
			if _, err := toml.Decode(cm.Data[utils.OpenShiftMCPServerConfigFilename], &config); err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(baseToolsets[:])
			if tc.want {
				want = append(want, "netobserv")
			}
			if strings.Join(config.Toolsets, ",") != strings.Join(want, ",") {
				t.Fatalf("got %v want %v", config.Toolsets, want)
			}
			for _, setting := range []string{`kind = "Secret"`, `group = "rbac.authorization.k8s.io"`, `guardrails = "!tsdb"`} {
				if !strings.Contains(cm.Data[utils.OpenShiftMCPServerConfigFilename], setting) {
					t.Fatalf("lost setting %s", setting)
				}
			}
			if strings.Contains(cm.Data[utils.OpenShiftMCPServerConfigFilename], "toolset_configs.netobserv") {
				t.Fatal("added backend configuration")
			}
			rv := cm.ResourceVersion
			if err := ReconcileResources(r, ctx, cr); err != nil {
				t.Fatal(err)
			}
			if err := c.Get(ctx, key, cm); err != nil {
				t.Fatal(err)
			}
			if cm.ResourceVersion != rv {
				t.Fatal("unchanged decision rewrote configuration")
			}
			// Configuration repair must not prevent independent MCP resources.
			if err := c.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerServiceAccountName, Namespace: utils.OLSNamespaceDefault}, &corev1.ServiceAccount{}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNetObservState(t *testing.T) {
	for _, present := range []bool{false, true} {
		name := "absent"
		prior := `toolsets = ["core"]`
		opposite := `toolsets = ["core", "netobserv"]`
		if present {
			name = "present"
			prior, opposite = opposite, prior
		}
		t.Run(name, func(t *testing.T) {
			configMap := func(config string) *corev1.ConfigMap {
				return &corev1.ConfigMap{Data: map[string]string{utils.OpenShiftMCPServerConfigFilename: config}}
			}
			assertDecision := func(ctx context.Context, existing *corev1.ConfigMap, want bool) {
				t.Helper()
				got, err := netObservEnabled(ctx, existing)
				if err != nil || got != want {
					t.Fatalf("got present=%v err=%v; want present=%v", got, err, want)
				}
			}
			var state NetObservState
			// A confirmed decision overrides persisted configuration and survives
			// conflicting, missing, or malformed configuration during uncertainty.
			assertDecision(state.WithPresence(context.Background(), present, true), configMap(opposite), present)
			unknownCtx := state.WithPresence(context.Background(), !present, false)
			assertDecision(unknownCtx, configMap(opposite), present)
			assertDecision(unknownCtx, nil, present)
			assertDecision(unknownCtx, configMap(`toolsets = [`), present)
			// A new confirmed decision must supersede the remembered one.
			assertDecision(state.WithPresence(context.Background(), !present, true), configMap(prior), !present)
			assertDecision(state.WithPresence(context.Background(), present, false), nil, !present)

			// Restart recovery initializes memory for both presence and absence.
			var restarted NetObservState
			unknownCtx = restarted.WithPresence(context.Background(), !present, false)
			assertDecision(unknownCtx, configMap(prior), present)
			assertDecision(restarted.WithPresence(context.Background(), !present, false), nil, present)
		})
	}
}
