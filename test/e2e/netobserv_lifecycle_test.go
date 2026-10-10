package e2e

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/BurntSushi/toml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/ocpmcp"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// This fixture is opt-in for a dedicated cluster. It never replaces an existing
// product CRD and never creates or reads FlowCollector instances or configures a
// backend. Cleanup checks ownership, unchanged fixture spec, and UID preconditions.
var _ = Describe("Controlled NetObserv API lifecycle", Ordered, Serial, Label("NetObservLifecycle"), func() {
	It("enables and removes NetObserv on API events without OLSConfig edits or a backend", func() {
		if os.Getenv("OLS_E2E_NETOBSERV_LIFECYCLE") != "true" {
			Skip("requires OLS_E2E_NETOBSERV_LIFECYCLE=true and an explicitly approved dedicated cluster")
		}
		c, err := GetClient(nil)
		Expect(err).NotTo(HaveOccurred())
		config := rest.CopyConfig(c.config)
		config.Timeout = DefaultClientTimeout
		extensions, err := apiextensionsclient.NewForConfig(config)
		Expect(err).NotTo(HaveOccurred())
		definitions := extensions.ApiextensionsV1().CustomResourceDefinitions()
		ctx := context.Background()
		_, err = definitions.Get(ctx, ocpmcp.FlowCollectorCRDName, metav1.GetOptions{})
		if err == nil {
			Skip("an existing FlowCollector CRD is present; it must not be modified")
		}
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "cannot establish that the fixture CRD is absent: %v", err)

		cr, err := generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		cr.Spec.OLSConfig.IntrospectionEnabled = utils.BoolPtr(true)
		cr.Spec.OLSConfig.ByokRAGOnly = true
		err = c.Create(cr)
		if apierrors.IsAlreadyExists(err) {
			Skip("an existing OLSConfig is present; this suite only modifies its own fixture")
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(cr.UID).NotTo(BeEmpty())
		Expect(cr.Generation).To(BeNumerically(">", 0))
		olsUID := cr.UID
		DeferCleanup(func() {
			current := &olsv1alpha1.OLSConfig{ObjectMeta: metav1.ObjectMeta{Name: cr.Name}}
			if err := c.Get(current); apierrors.IsNotFound(err) {
				return
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(current.UID).To(Equal(olsUID), "refusing to delete another OLSConfig")
			Expect(c.kClient.Delete(ctx, current, ctrlclient.Preconditions{UID: &olsUID})).To(Succeed())
			Eventually(func() bool { return apierrors.IsNotFound(c.Get(current)) }, 3*time.Minute, DefaultPollInterval).Should(BeTrue())
		})
		olsGeneration := cr.Generation
		deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerDeploymentName, Namespace: OLSNameSpace}}
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: OLSNameSpace}}
		readToolsets := func(g Gomega) []string {
			g.Expect(c.Get(cm)).To(Succeed())
			var parsed struct {
				Toolsets []string `toml:"toolsets"`
			}
			_, err := toml.Decode(cm.Data[utils.OpenShiftMCPServerConfigFilename], &parsed)
			g.Expect(err).NotTo(HaveOccurred())
			return parsed.Toolsets
		}
		waitForState := func(expected []string, previousGeneration int64) {
			Eventually(func(g Gomega) {
				g.Expect(readToolsets(g)).To(Equal(expected))
				g.Expect(c.Get(deployment)).To(Succeed())
				g.Expect(deployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]).To(Equal(cm.ResourceVersion))
				g.Expect(deployment.Generation).To(BeNumerically(">", previousGeneration))
			}, DefaultPollTimeout, DefaultPollInterval).Should(Succeed())
			Expect(c.WaitForDeploymentRollout(deployment)).To(Succeed())
			Expect(c.Get(cr)).To(Succeed())
			Expect(cr.UID).To(Equal(olsUID))
			Expect(cr.Generation).To(Equal(olsGeneration), "API events must not require OLSConfig edits")
		}

		By("starting the base server before the API exists")
		Expect(c.WaitForDeploymentRollout(deployment)).To(Succeed())
		var base []string
		Eventually(func(g Gomega) {
			base = readToolsets(g)
			g.Expect(base).NotTo(ContainElement("netobserv"))
			g.Expect(c.Get(deployment)).To(Succeed())
			g.Expect(deployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]).To(Equal(cm.ResourceVersion))
		}, DefaultPollTimeout, DefaultPollInterval).Should(Succeed())
		base = slices.Clone(base)
		baseGeneration := deployment.Generation
		Expect(netObservMCPToolNames(c)).To(ContainElements("namespaces_list", "pods_list"))

		By("installing only a labeled, minimal FlowCollector API fixture")
		const fixtureLabel = "ols.openshift.io/netobserv-e2e-fixture"
		version := func(name string, storage bool) apiextensionsv1.CustomResourceDefinitionVersion {
			return apiextensionsv1.CustomResourceDefinitionVersion{
				Name: name, Served: true, Storage: storage,
				Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object"}},
			}
		}
		fixture, err := definitions.Create(ctx, &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: ocpmcp.FlowCollectorCRDName, Labels: map[string]string{fixtureLabel: "OLS-4399"}},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: ocpmcp.FlowCollectorAPIGroup, Scope: apiextensionsv1.ClusterScoped,
				Names:    apiextensionsv1.CustomResourceDefinitionNames{Plural: "flowcollectors", Singular: "flowcollector", Kind: "FlowCollector", ListKind: "FlowCollectorList"},
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{version("v1beta1", false), version("v1beta2", true)},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		fixtureUID := fixture.UID
		expectedFixtureSpec := fixture.Spec.DeepCopy()
		removeFixture := func() {
			current, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(current.UID).To(Equal(fixtureUID), "refusing to delete a replacement CRD")
			Expect(current.Labels[fixtureLabel]).To(Equal("OLS-4399"), "fixture ownership changed")
			Expect(current.OwnerReferences).To(BeEmpty(), "fixture was adopted by another controller")
			Expect(current.Spec).To(Equal(*expectedFixtureSpec), "fixture spec changed outside this test")
			Expect(deleteNetObservCRDFixture(ctx, definitions, current)).To(Succeed())
			Eventually(func() bool {
				_, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
				return apierrors.IsNotFound(err)
			}, DefaultPollTimeout, DefaultPollInterval).Should(BeTrue())
		}
		DeferCleanup(removeFixture)
		Eventually(func(g Gomega) {
			current, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(current.Status.Conditions).To(ContainElement(And(
				HaveField("Type", apiextensionsv1.Established), HaveField("Status", apiextensionsv1.ConditionTrue),
			)))
		}, DefaultPollTimeout, DefaultPollInterval).Should(Succeed())
		GinkgoWriter.Printf("Owned FlowCollector fixture UID: %s\n", fixtureUID)

		By("observing automatic enablement and listing NetObserv tools without a backend")
		waitForState(append(slices.Clone(base), "netobserv"), baseGeneration)
		Expect(netObservMCPToolNames(c)).To(ContainElements(
			"namespaces_list", "pods_list", "netobserv_export_flows", "netobserv_get_flow_metrics", "netobserv_list_flows",
		))
		enabledGeneration, enabledResourceVersion := deployment.Generation, cm.ResourceVersion

		By("keeping NetObserv enabled when only another API version remains served")
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			Expect(current.UID).To(Equal(fixtureUID))
			Expect(current.Spec).To(Equal(*expectedFixtureSpec))
			current.Spec.Versions[1].Served = false
			updated, err := definitions.Update(ctx, current, metav1.UpdateOptions{})
			if err == nil {
				expectedFixtureSpec = updated.Spec.DeepCopy()
			}
			return err
		})).To(Succeed())
		Eventually(func() bool {
			_, err := extensions.Discovery().ServerResourcesForGroupVersion(ocpmcp.FlowCollectorAPIGroup + "/v1beta2")
			return apierrors.IsNotFound(err)
		}, DefaultPollTimeout, DefaultPollInterval).Should(BeTrue())
		Consistently(func(g Gomega) {
			g.Expect(readToolsets(g)).To(Equal(append(slices.Clone(base), "netobserv")))
			g.Expect(cm.ResourceVersion).To(Equal(enabledResourceVersion))
			g.Expect(c.Get(deployment)).To(Succeed())
			g.Expect(deployment.Generation).To(Equal(enabledGeneration))
		}, 15*time.Second, time.Second).Should(Succeed())

		By("refusing cleanup when the fixture changes after the guarded read")
		staleFixture, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
			current, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			Expect(current.UID).To(Equal(fixtureUID))
			if current.Annotations == nil {
				current.Annotations = make(map[string]string)
			}
			current.Annotations["ols.openshift.io/netobserv-cleanup-race"] = "changed-after-read"
			_, err = definitions.Update(ctx, current, metav1.UpdateOptions{})
			return err
		})).To(Succeed())
		err = deleteNetObservCRDFixture(ctx, definitions, staleFixture)
		Expect(apierrors.IsConflict(err)).To(BeTrue(), "stale cleanup must fail closed: %v", err)
		current, err := definitions.Get(ctx, fixture.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(current.UID).To(Equal(fixtureUID))

		By("removing only the owned API fixture and observing automatic disablement")
		removeFixture()
		waitForState(base, enabledGeneration)
		names := netObservMCPToolNames(c)
		Expect(names).To(ContainElements("namespaces_list", "pods_list"))
		for _, name := range names {
			Expect(name).NotTo(HavePrefix("netobserv_"))
		}
	})
})

// The delete must apply to the same snapshot whose ownership and spec were checked.
func deleteNetObservCRDFixture(ctx context.Context, definitions apiextensionsv1client.CustomResourceDefinitionInterface, current *apiextensionsv1.CustomResourceDefinition) error {
	return definitions.Delete(ctx, current.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{UID: &current.UID, ResourceVersion: &current.ResourceVersion},
	})
}
