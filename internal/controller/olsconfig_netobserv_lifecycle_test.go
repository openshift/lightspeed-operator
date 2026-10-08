package controller

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/ocpmcp"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/yaml"
)

// These CRDs exist only inside envtest. Never install/delete these fixtures on a
// shared cluster: the real FlowCollector CRD may belong to an installed operator.
func flowCollectorTestCRD(name, group, kind string) *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "flowcollectors", Singular: "flowcollector", Kind: kind},
			Scope: apiextensionsv1.ClusterScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{Name: "v1beta1", Served: true, Storage: true, Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"}}},
				{Name: "v1beta2", Served: true, Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"}}},
			},
		},
	}
}

// Observe production watch routing without reconciling unrelated operands. The
// controller sees no OLSConfig, so a queued request terminates after this Get.
// List/watch traffic and CRD cache selection still use the real envtest server.
type netObservWatchClient struct {
	client.Client
	requests atomic.Int64
}

func (c *netObservWatchClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*olsv1alpha1.OLSConfig); ok && key.Name == utils.OLSConfigName {
		c.requests.Add(1)
		return apierrors.NewNotFound(schema.GroupResource{Group: olsv1alpha1.GroupVersion.Group, Resource: "olsconfigs"}, key.Name)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

var _ = Describe("NetObserv API lifecycle integration", func() {
	var (
		r      *OLSConfigReconciler
		cr     *olsv1alpha1.OLSConfig
		crd    *apiextensionsv1.CustomResourceDefinition
		cmKey  client.ObjectKey
		depKey client.ObjectKey
	)

	BeforeEach(func() {
		// Use a dedicated namespace because envtest does not garbage-collect
		// resources whose owner reference points at our synthetic OLSConfig.
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "netobserv-lifecycle-"}}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		d, err := discovery.NewDiscoveryClientForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())
		r = &OLSConfigReconciler{Client: k8sClient, APIReader: k8sClient, Logger: logr.Discard(), Options: getDefaultReconcilerOptions(ns.Name), DiscoveryClient: d}
		r.Options.OpenShiftMCPServerImage = "test-mcp-image"
		cr = utils.GetDefaultOLSConfigCR()
		cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
		crd = flowCollectorTestCRD(ocpmcp.FlowCollectorCRDName, ocpmcp.FlowCollectorAPIGroup, "FlowCollector")
		cmKey = client.ObjectKey{Namespace: ns.Name, Name: utils.OpenShiftMCPServerConfigCmName}
		depKey = client.ObjectKey{Namespace: ns.Name, Name: utils.OpenShiftMCPServerDeploymentName}
		secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: utils.OpenShiftMCPServerCertsSecretName}, Data: map[string][]byte{"tls.crt": []byte("test-cert"), "tls.key": []byte("test-key")}}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		cleanupReconciler := &OLSConfigReconciler{Client: k8sClient, APIReader: k8sClient, Logger: logr.Discard(), Options: r.Options}
		DeferCleanup(func() {
			// Cleanup always uses the direct client, even after a test manager stops.
			// Envtest has neither namespace nor GC controllers.
			Expect(ocpmcp.Remove(cleanupReconciler, ctx)).To(Succeed())
			_ = k8sClient.Delete(ctx, secret)
			_ = k8sClient.Delete(ctx, ns)
		})
		DeferCleanup(func() {
			err := k8sClient.Delete(ctx, crd)
			Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue())
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), &apiextensionsv1.CustomResourceDefinition{}))
			}, 15*time.Second, 100*time.Millisecond).Should(BeTrue())
		})
	})

	reconcileMCP := func() error {
		presenceCtx, err := r.netObservContext(ctx, cr)
		if err != nil {
			return err
		}
		if err := ocpmcp.ReconcileResources(r, presenceCtx, cr); err != nil {
			return err
		}
		return ocpmcp.ReconcileDeployment(r, presenceCtx, cr)
	}
	configuration := func() (*corev1.ConfigMap, *appsv1.Deployment, bool) {
		cm := &corev1.ConfigMap{}
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, cmKey, cm)).To(Succeed())
		Expect(k8sClient.Get(ctx, depKey, dep)).To(Succeed())
		var parsed struct {
			Toolsets []string `toml:"toolsets"`
		}
		_, err := toml.Decode(cm.Data[utils.OpenShiftMCPServerConfigFilename], &parsed)
		Expect(err).NotTo(HaveOccurred())
		count := 0
		for _, name := range parsed.Toolsets {
			if name == "netobserv" {
				count++
			}
		}
		Expect(count).To(BeNumerically("<=", 1))
		Expect(dep.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]).To(Equal(cm.ResourceVersion))
		return cm, dep, count == 1
	}

	It("enables on installation, retains any served version, rolls on removal, and does not churn", func() {
		Expect(reconcileMCP()).To(Succeed())
		baseCM, baseDep, enabled := configuration()
		Expect(enabled).To(BeFalse())
		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		Eventually(reconcileMCP, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		enabledCM, enabledDep, enabled := configuration()
		Expect(enabled).To(BeTrue())
		Expect(enabledCM.ResourceVersion).NotTo(Equal(baseCM.ResourceVersion))
		Expect(enabledDep.Generation).To(BeNumerically(">", baseDep.Generation))
		Expect(enabledDep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]).NotTo(BeEmpty())

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), crd)).To(Succeed())
		before := crd.DeepCopy()
		crd.Spec.Versions[0].Served = false // The non-storage served version still qualifies.
		Expect(k8sClient.Patch(ctx, crd, client.MergeFrom(before))).To(Succeed())
		Eventually(reconcileMCP, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		unchangedCM, unchangedDep, enabled := configuration()
		Expect(enabled).To(BeTrue())
		Expect(unchangedCM.ResourceVersion).To(Equal(enabledCM.ResourceVersion))
		Expect(unchangedDep.ResourceVersion).To(Equal(enabledDep.ResourceVersion))

		Expect(k8sClient.Delete(ctx, crd)).To(Succeed())
		Eventually(reconcileMCP, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		removedCM, removedDep, enabled := configuration()
		Expect(enabled).To(BeFalse())
		Expect(removedCM.ResourceVersion).NotTo(Equal(enabledCM.ResourceVersion))
		Expect(removedDep.Generation).To(BeNumerically(">", enabledDep.Generation))
		Expect(reconcileMCP()).To(Succeed())
		stableCM, stableDep, _ := configuration()
		Expect(stableCM.ResourceVersion).To(Equal(removedCM.ResourceVersion))
		Expect(stableDep.ResourceVersion).To(Equal(removedDep.ResourceVersion))
	})

	It("repairs malformed configuration and runs Phase 2 before discovery retry, with disablement taking precedence", func() {
		// The full controller needs fixtures for unrelated operands. A fake
		// client keeps this test focused on phase sequencing, not TLS issuance.
		cr.Finalizers = []string{utils.OLSConfigFinalizer}
		cr.Spec.OLSConfig.ByokRAGOnly = true
		c := fake.NewClientBuilder().WithScheme(k8sClient.Scheme()).WithStatusSubresource(cr).WithObjects(
			cr,
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: utils.OperatorDeploymentName, Namespace: r.GetNamespace(), UID: "test-operator"}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: r.GetNamespace()}, Data: map[string][]byte{"apitoken": []byte("test-token")}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerCertsSecretName, Namespace: r.GetNamespace()}, Data: map[string][]byte{"tls.crt": []byte("test-cert"), "tls.key": []byte("test-key")}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: r.GetNamespace()}, Data: map[string]string{utils.OpenShiftMCPServerConfigFilename: `toolsets = [`}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OLSCAConfigMap, Namespace: r.GetNamespace()}, Data: map[string]string{utils.AppOtelCollectorCACertFile: utils.TestCACert}},
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: r.GetNamespace()}, Data: map[string]string{"ca.crt": utils.TestCACert}},
		).Build()
		for _, name := range []string{utils.ConsoleUIServiceCertSecretName, utils.OtelCollectorCertsSecretName, utils.PostgresCertsSecretName, utils.OLSCertsSecretName} {
			Expect(c.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.GetNamespace()}, Data: map[string][]byte{"tls.crt": []byte("test-cert"), "tls.key": []byte("test-key")}})).To(Succeed())
		}
		r.Client, r.APIReader = c, c
		outage := errors.New("discovery unavailable")
		r.DiscoveryClient = controllerDiscoveryStub{err: outage}
		reconcileCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, err := r.Reconcile(reconcileCtx, ctrl.Request{NamespacedName: client.ObjectKey{Name: cr.Name}})
		Expect(errors.Is(err, outage)).To(BeTrue(), "discovery uncertainty must remain retryable: %v", err)
		cm := &corev1.ConfigMap{}
		Expect(c.Get(ctx, cmKey, cm)).To(Succeed())
		var parsed struct {
			Toolsets []string `toml:"toolsets"`
		}
		_, parseErr := toml.Decode(cm.Data[utils.OpenShiftMCPServerConfigFilename], &parsed)
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(parsed.Toolsets).To(Equal([]string{"core", "config", "helm", "observability/metrics", "kubevirt"}))
		Expect(c.Get(ctx, depKey, &appsv1.Deployment{})).To(Succeed(), "discovery must not stop base deployment reconciliation: %v", err)

		// A CRD-triggered request must not resurrect managed resources after
		// the user disables introspection, even if discovery is still failing.
		Expect(c.Get(ctx, client.ObjectKeyFromObject(cr), cr)).To(Succeed())
		cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(false)
		Expect(c.Update(ctx, cr)).To(Succeed())
		Expect(c.Create(ctx, crd)).To(Succeed())
		_, err = r.Reconcile(reconcileCtx, flowCollectorCRDRequests(ctx, crd)[0])
		Expect(errors.Is(err, outage)).To(BeFalse())
		Expect(apierrors.IsNotFound(c.Get(ctx, cmKey, &corev1.ConfigMap{}))).To(BeTrue())
		Expect(apierrors.IsNotFound(c.Get(ctx, depKey, &appsv1.Deployment{}))).To(BeTrue())
	})

	It("deploys base-only resources on a first-discovery outage and enables after recovery", func() {
		d := r.DiscoveryClient
		r.DiscoveryClient = controllerDiscoveryStub{err: errors.New("discovery unavailable")}
		unknownCtx, err := r.netObservContext(ctx, cr)
		Expect(err).To(MatchError(ContainSubstring("discovery unavailable")))
		Expect(ocpmcp.ReconcileResources(r, unknownCtx, cr)).To(Succeed())
		Expect(ocpmcp.ReconcileDeployment(r, unknownCtx, cr)).To(Succeed())
		_, _, enabled := configuration()
		Expect(enabled).To(BeFalse())
		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		r.DiscoveryClient = d
		Eventually(reconcileMCP, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		_, _, enabled = configuration()
		Expect(enabled).To(BeTrue())
	})

	It("retains the last successful config and rollout after restart during a discovery outage", func() {
		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		Eventually(reconcileMCP, 15*time.Second, 100*time.Millisecond).Should(Succeed())
		beforeCM, beforeDep, enabled := configuration()
		Expect(enabled).To(BeTrue())
		// A fresh reconciler has no in-memory presence state. Only the persisted
		// ConfigMap can preserve the successful result through this outage.
		r = &OLSConfigReconciler{Client: k8sClient, APIReader: k8sClient, Logger: logr.Discard(), Options: r.Options, DiscoveryClient: controllerDiscoveryStub{err: errors.New("discovery unavailable")}}
		unknownCtx, err := r.netObservContext(ctx, cr)
		Expect(err).To(MatchError(ContainSubstring("discovery unavailable")))
		Expect(ocpmcp.ReconcileResources(r, unknownCtx, cr)).To(Succeed())
		Expect(ocpmcp.ReconcileDeployment(r, unknownCtx, cr)).To(Succeed())
		afterCM, afterDep, enabled := configuration()
		Expect(enabled).To(BeTrue())
		Expect(afterCM.ResourceVersion).To(Equal(beforeCM.ResourceVersion))
		Expect(afterDep.ResourceVersion).To(Equal(beforeDep.ResourceVersion))
	})

	for _, restart := range []bool{false, true} {
		for _, corrupt := range []bool{false, true} {
			name := "retains the discovered decision"
			if restart {
				name = "retains the decision recovered from persisted configuration after restart"
			}
			if corrupt {
				name += " when configuration is corrupted during an outage"
			} else {
				name += " when configuration is deleted during an outage"
			}
			It(name, func() {
				Expect(k8sClient.Create(ctx, crd)).To(Succeed())
				Eventually(reconcileMCP, 15*time.Second, 100*time.Millisecond).Should(Succeed())
				beforeCM, _, enabled := configuration()
				originalConfig := beforeCM.Data[utils.OpenShiftMCPServerConfigFilename]
				Expect(enabled).To(BeTrue())
				outage := errors.New("discovery unavailable")
				if restart {
					r = &OLSConfigReconciler{Client: k8sClient, APIReader: k8sClient, Logger: logr.Discard(), Options: r.Options}
				}
				r.DiscoveryClient = controllerDiscoveryStub{err: outage}
				unknownCtx, err := r.netObservContext(ctx, cr)
				Expect(errors.Is(err, outage)).To(BeTrue())
				// Recover the persisted decision before testing subsequent loss.
				Expect(ocpmcp.ReconcileResources(r, unknownCtx, cr)).To(Succeed())
				if corrupt {
					beforeCM.Data[utils.OpenShiftMCPServerConfigFilename] = `toolsets = [`
					Expect(k8sClient.Update(ctx, beforeCM)).To(Succeed())
				} else {
					Expect(k8sClient.Delete(ctx, beforeCM)).To(Succeed())
				}
				unknownCtx, err = r.netObservContext(ctx, cr)
				Expect(errors.Is(err, outage)).To(BeTrue())
				Expect(ocpmcp.ReconcileResources(r, unknownCtx, cr)).To(Succeed())
				Expect(ocpmcp.ReconcileDeployment(r, unknownCtx, cr)).To(Succeed())
				afterCM, afterDep, enabled := configuration()
				Expect(enabled).To(BeTrue())
				Expect(afterCM.Data[utils.OpenShiftMCPServerConfigFilename]).To(Equal(originalConfig))
				Expect(ocpmcp.ReconcileResources(r, unknownCtx, cr)).To(Succeed())
				Expect(ocpmcp.ReconcileDeployment(r, unknownCtx, cr)).To(Succeed())
				stableCM, stableDep, enabled := configuration()
				Expect(enabled).To(BeTrue())
				Expect(stableCM.ResourceVersion).To(Equal(afterCM.ResourceVersion))
				Expect(stableDep.ResourceVersion).To(Equal(afterDep.ResourceVersion))
			})
		}
	}

	It("authorizes named CRD get/list/watch with the generated RBAC, but not unrestricted access", func() {
		data, err := os.ReadFile("../../config/rbac/role.yaml")
		Expect(err).NotTo(HaveOccurred())
		generated := &rbacv1.ClusterRole{}
		Expect(yaml.Unmarshal(data, generated)).To(Succeed())
		role := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: r.GetNamespace()}}
		for _, rule := range generated.Rules {
			if len(rule.APIGroups) == 1 && rule.APIGroups[0] == apiextensionsv1.GroupName {
				role.Rules = append(role.Rules, rule)
			}
		}
		Expect(role.Rules).To(HaveLen(1))
		Expect(role.Rules[0].ResourceNames).To(Equal([]string{ocpmcp.FlowCollectorCRDName}))
		Expect(role.Rules[0].Resources).To(Equal([]string{"customresourcedefinitions"}))
		Expect(role.Rules[0].Verbs).To(ConsistOf("get", "list", "watch"))
		user, err := testEnv.AddUser(envtest.User{Name: r.GetNamespace()}, cfg)
		Expect(err).NotTo(HaveOccurred())
		binding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: role.Name},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role.Name},
			Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: r.GetNamespace()}},
		}
		Expect(k8sClient.Create(ctx, role)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, role) })
		Expect(k8sClient.Create(ctx, binding)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, binding) })
		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		restricted, err := apiextensionsclient.NewForConfig(user.Config())
		Expect(err).NotTo(HaveOccurred())
		api := restricted.ApiextensionsV1().CustomResourceDefinitions()
		Eventually(func() error {
			_, err := api.Get(ctx, ocpmcp.FlowCollectorCRDName, metav1.GetOptions{})
			return err
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		opts := metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", ocpmcp.FlowCollectorCRDName).String()}
		list, err := api.List(ctx, opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))
		watch, err := api.Watch(ctx, opts)
		Expect(err).NotTo(HaveOccurred())
		watch.Stop()
		_, err = api.List(ctx, metav1.ListOptions{})
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
		_, err = api.Get(ctx, "unrelated.example", metav1.GetOptions{})
		Expect(apierrors.IsForbidden(err)).To(BeTrue())
	})

	It("registers a name-filtered production watch that enqueues installation, updates and deletion without polling", func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: k8sClient.Scheme(), Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
			Cache: cache.Options{ByObject: map[client.Object]cache.ByObject{
				&apiextensionsv1.CustomResourceDefinition{}: {Field: fields.OneTermEqualSelector("metadata.name", ocpmcp.FlowCollectorCRDName)},
			}},
		})
		Expect(err).NotTo(HaveOccurred())
		observed := &netObservWatchClient{Client: mgr.GetClient()}
		r.Client = observed
		r.APIReader = mgr.GetAPIReader()
		Expect(r.SetupWithManager(mgr)).To(Succeed())
		managerCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- mgr.Start(managerCtx) }()
		DeferCleanup(func() {
			cancel()
			Eventually(done, 10*time.Second).Should(Receive(BeNil()))
		})
		cacheCtx, cacheCancel := context.WithTimeout(managerCtx, 15*time.Second)
		defer cacheCancel()
		Expect(mgr.GetCache().WaitForCacheSync(cacheCtx)).To(BeTrue())
		// Startup events from the existing ClusterVersion and TLS Secret can
		// also read OLSConfig. Wait for those lookups to settle first.
		last := observed.requests.Load()
		stable := 0
		Eventually(func() bool {
			current := observed.requests.Load()
			if current != last {
				last, stable = current, 0
			} else {
				stable++
			}
			return stable >= 10
		}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
		initial := observed.requests.Load()
		unrelated := flowCollectorTestCRD("flowcollectors.unrelated.example", "unrelated.example", "OtherFlowCollector")
		Expect(k8sClient.Create(ctx, unrelated)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, unrelated) })
		Consistently(observed.requests.Load, time.Second, 100*time.Millisecond).Should(Equal(initial))

		Expect(k8sClient.Create(ctx, crd)).To(Succeed())
		Eventually(observed.requests.Load, 10*time.Second).Should(BeNumerically(">", initial))
		Eventually(func() error {
			return mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(crd), &apiextensionsv1.CustomResourceDefinition{})
		}, 10*time.Second).Should(Succeed())
		Expect(apierrors.IsNotFound(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(unrelated), &apiextensionsv1.CustomResourceDefinition{}))).To(BeTrue())
		// Let the CRD establishment/status event settle before testing updates.
		Eventually(func() error { return k8sClient.Get(ctx, client.ObjectKeyFromObject(crd), crd) }, 10*time.Second).Should(Succeed())
		Consistently(func() bool {
			return apierrors.IsNotFound(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(unrelated), &apiextensionsv1.CustomResourceDefinition{}))
		}, time.Second, 100*time.Millisecond).Should(BeTrue())
		beforeUpdate := observed.requests.Load()
		before := crd.DeepCopy()
		crd.Spec.Versions[0].Served = false
		Expect(k8sClient.Patch(ctx, crd, client.MergeFrom(before))).To(Succeed())
		Eventually(observed.requests.Load, 10*time.Second).Should(BeNumerically(">", beforeUpdate))
		beforeDelete := observed.requests.Load()
		Expect(k8sClient.Delete(ctx, crd)).To(Succeed())
		Eventually(observed.requests.Load, 10*time.Second).Should(BeNumerically(">", beforeDelete))
	})
})
