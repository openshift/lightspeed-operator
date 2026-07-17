package otelcollector

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func expectOwnedByOLSConfig(obj metav1.Object) {
	olsConfig := &olsv1alpha1.OLSConfig{}
	Expect(k8sClient.Get(ctx, crNamespacedName, olsConfig)).To(Succeed())

	var ownerRef *metav1.OwnerReference
	for i := range obj.GetOwnerReferences() {
		ref := &obj.GetOwnerReferences()[i]
		if ref.APIVersion == utils.OLSConfigAPIVersion &&
			ref.Kind == utils.OLSConfigKind &&
			ref.Name == olsConfig.Name {
			ownerRef = ref
			break
		}
	}
	Expect(ownerRef).NotTo(BeNil(), "expected %T %s to be owned by OLSConfig", obj, obj.GetName())
	Expect(ownerRef.Name).To(Equal(olsConfig.Name))
}

var _ = Describe("OTEL Collector reconciler", Ordered, func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeAll(func() {
		testCR = cr.DeepCopy()
		ensurePostgresSecret()
	})

	Context("Phase 1 resources", func() {
		BeforeAll(func() {
			err := ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the collector ConfigMap", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(cm)
			Expect(cm.Data[utils.OtelCollectorConfigMapDataKey]).To(ContainSubstring("routing/logs"))
		})

		It("should create the collector ServiceAccount", func() {
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}, sa)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(sa)
		})

		It("should create the collector Postgres DSN Secret", func() {
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorPostgresDSNSecretName,
				Namespace: utils.OLSNamespaceDefault,
			}, secret)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(secret)
			Expect(secret.Data).To(HaveKey(utils.OtelCollectorPostgresConnectionStringSecretKey))
		})

		It("should create the collector NetworkPolicy", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(np)
		})

		It("should skip ConfigMap update when data is unchanged", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			oldRV := cm.ResourceVersion

			err = ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ResourceVersion).To(Equal(oldRV))
		})
	})

	Context("Phase 2 deployment", func() {
		BeforeAll(func() {
			ensureCollectorTLSSecret()
			err := ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the collector Service", func() {
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(svc)
			Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OtelCollectorCertsSecretName))
		})

		It("should create the client connectivity ConfigMap with CA from the serving cert", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(cm)

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorCertsSecretName,
				Namespace: utils.OLSNamespaceDefault,
			}, secret)).To(Succeed())

			host := utils.OtelCollectorServiceName + "." + utils.OLSNamespaceDefault + ".svc"
			Expect(cm.Data[utils.OtelCollectorClientCollectorEndpointKey]).To(Equal(
				fmt.Sprintf("%s:%d", host, utils.OtelCollectorGRPCPort),
			))
			Expect(cm.Data[utils.OtelCollectorClientAdminEndpointKey]).To(Equal(
				fmt.Sprintf("https://%s:%d", host, utils.OtelCollectorAdminPort),
			))
			Expect(cm.Data[utils.OtelCollectorClientCACertKey]).To(Equal(string(secret.Data["tls.crt"])))
			Expect(cm.Data).NotTo(HaveKey(utils.OtelCollectorClientCredentialsSecretKey))
		})

		It("should skip client ConfigMap update when data is unchanged", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			oldRV := cm.ResourceVersion

			err = ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.ResourceVersion).To(Equal(oldRV))
		})

		It("should restore client ConfigMap labels and owner references when data is unchanged", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Labels).NotTo(BeEmpty())
			Expect(cm.OwnerReferences).NotTo(BeEmpty())

			cm.Labels = map[string]string{"drifted": "true"}
			cm.OwnerReferences = nil
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			err = ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Labels).To(Equal(utils.GenerateOtelCollectorSelectorLabels()))
			expectOwnedByOLSConfig(cm)
		})

		It("should refresh client ConfigMap CA via RestartOtelCollector", func() {
			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			cm.Data[utils.OtelCollectorClientCACertKey] = "stale-ca"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)).To(Succeed())

			err = RestartOtelCollector(testReconcilerInstance, ctx, dep)
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorCertsSecretName,
				Namespace: utils.OLSNamespaceDefault,
			}, secret)).To(Succeed())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data[utils.OtelCollectorClientCACertKey]).To(Equal(string(secret.Data["tls.crt"])))
		})

		It("should skip Service update when spec and serving-cert annotation are unchanged", func() {
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			oldRV := svc.ResourceVersion

			err = ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.ResourceVersion).To(Equal(oldRV))
		})

		It("should heal a missing serving-cert annotation on the collector Service", func() {
			svc := &corev1.Service{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			delete(svc.Annotations, utils.ServingCertSecretAnnotationKey)
			Expect(k8sClient.Update(ctx, svc)).To(Succeed())

			err = ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceName,
				Namespace: utils.OLSNamespaceDefault,
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Annotations[utils.ServingCertSecretAnnotationKey]).To(Equal(utils.OtelCollectorCertsSecretName))
		})

		It("should create the collector Deployment", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(dep)
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.OtelCollectorImageDefault))
			Expect(dep.Annotations).To(HaveKey(utils.OtelCollectorConfigMapResourceVersionAnnotation))
		})

		It("should skip Deployment update when spec and ConfigMap version are unchanged", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			oldRV := dep.ResourceVersion
			oldForceReload := dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]

			err = ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.ResourceVersion).To(Equal(oldRV))
			Expect(dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]).To(Equal(oldForceReload))
		})

		It("should trigger a rolling restart when the collector ConfigMap changes", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			oldCMVersion := dep.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation]
			Expect(oldCMVersion).NotTo(BeEmpty())

			updatedCR := testCR.DeepCopy()
			updatedCR.Spec.Audit.TracingEndpoint = "tempo:4317"
			err = ReconcileOtelCollectorResources(testReconcilerInstance, ctx, updatedCR)
			Expect(err).NotTo(HaveOccurred())

			err = ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, updatedCR)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())
			Expect(dep.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation]).NotTo(Equal(oldCMVersion))
			Expect(dep.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
		})

		It("should restart via RestartOtelCollector", func() {
			dep := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)
			Expect(err).NotTo(HaveOccurred())

			err = RestartOtelCollector(testReconcilerInstance, ctx, dep)
			Expect(err).NotTo(HaveOccurred())

			updated := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
		})
	})
})
