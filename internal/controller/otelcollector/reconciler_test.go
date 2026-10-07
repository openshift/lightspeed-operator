package otelcollector

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
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

func expectOtelDataverseExporterResourcesAbsent() {
	Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{
		Name:      utils.OtelDataverseExporterConfigMapName,
		Namespace: utils.OLSNamespaceDefault,
	}, &corev1.ConfigMap{}))).To(BeTrue())
	Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{
		Name: utils.OtelDataverseExporterClusterRoleName,
	}, &rbacv1.ClusterRole{}))).To(BeTrue())
	Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{
		Name: utils.OtelDataverseExporterClusterRoleBindingName,
	}, &rbacv1.ClusterRoleBinding{}))).To(BeTrue())
	Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{
		Name: utils.OtelDataverseExporterPullSecretClusterRoleName,
	}, &rbacv1.ClusterRole{}))).To(BeTrue())
	Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{
		Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
		Namespace: utils.TelemetryPullSecretNamespace,
	}, &rbacv1.RoleBinding{}))).To(BeTrue())
}

func expectOtelDataverseExporterResourcesPresent() {
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      utils.OtelDataverseExporterConfigMapName,
		Namespace: utils.OLSNamespaceDefault,
	}, &corev1.ConfigMap{})).To(Succeed())
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name: utils.OtelDataverseExporterClusterRoleName,
	}, &rbacv1.ClusterRole{})).To(Succeed())
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name: utils.OtelDataverseExporterClusterRoleBindingName,
	}, &rbacv1.ClusterRoleBinding{})).To(Succeed())
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name: utils.OtelDataverseExporterPullSecretClusterRoleName,
	}, &rbacv1.ClusterRole{})).To(Succeed())
	Expect(k8sClient.Get(ctx, types.NamespacedName{
		Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
		Namespace: utils.TelemetryPullSecretNamespace,
	}, &rbacv1.RoleBinding{})).To(Succeed())
}

var _ = Describe("OTEL Collector reconciler", Ordered, func() {
	var testCR *olsv1alpha1.OLSConfig

	BeforeAll(func() {
		testCR = cr.DeepCopy()
		testCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = false
		setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
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

		It("should create the OTEL exporter ConfigMap and least-privilege OpenShift auth RBAC", func() {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			expectOwnedByOLSConfig(cm)
			Expect(cm.Data[utils.OtelDataverseExporterConfigMapDataKey]).To(ContainSubstring("data_mode: otel"))
			Expect(cm.Data[utils.OtelDataverseExporterConfigMapDataKey]).NotTo(ContainSubstring("cloud.openshift.com"))

			clusterRole := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterClusterRoleName}, clusterRole)).To(Succeed())
			expectOwnedByOLSConfig(clusterRole)
			Expect(clusterRole.Rules).To(ConsistOf(rbacv1.PolicyRule{
				APIGroups:     []string{"config.openshift.io"},
				Resources:     []string{"clusterversions"},
				ResourceNames: []string{"version"},
				Verbs:         []string{"get"},
			}))

			clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterClusterRoleBindingName}, clusterRoleBinding)).To(Succeed())
			expectOwnedByOLSConfig(clusterRoleBinding)
			Expect(clusterRoleBinding.RoleRef.Name).To(Equal(utils.OtelDataverseExporterClusterRoleName))
			Expect(clusterRoleBinding.Subjects).To(ConsistOf(rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      utils.OtelCollectorServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}))

			pullSecretClusterRole := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterPullSecretClusterRoleName}, pullSecretClusterRole)).To(Succeed())
			expectOwnedByOLSConfig(pullSecretClusterRole)
			Expect(pullSecretClusterRole.Rules).To(ConsistOf(rbacv1.PolicyRule{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{utils.TelemetryPullSecretName},
				Verbs:         []string{"get"},
			}))

			pullSecretRoleBinding := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
				Namespace: utils.TelemetryPullSecretNamespace,
			}, pullSecretRoleBinding)).To(Succeed())
			expectOwnedByOLSConfig(pullSecretRoleBinding)
			Expect(pullSecretRoleBinding.RoleRef.Name).To(Equal(utils.OtelDataverseExporterPullSecretClusterRoleName))
			Expect(pullSecretRoleBinding.Subjects).To(ConsistOf(rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      utils.OtelCollectorServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}))
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
			Expect(np.Spec.Ingress).To(HaveLen(2))
		})

		It("should delete the legacy OTEL client ConfigMap on upgrade", func() {
			legacy := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.LegacyOtelCollectorClientConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{"collector-endpoint": "stale"},
			}
			Expect(k8sClient.Create(ctx, legacy)).To(Succeed())

			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())

			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.LegacyOtelCollectorClientConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, &corev1.ConfigMap{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
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
		It("should update exporter config and RBAC drift", func() {
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			cm.Data[utils.OtelDataverseExporterConfigMapDataKey] = "data_mode: json"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			clusterRole := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterClusterRoleName}, clusterRole)).To(Succeed())
			clusterRole.Rules = nil
			clusterRole.AggregationRule = &rbacv1.AggregationRule{
				ClusterRoleSelectors: []metav1.LabelSelector{
					{MatchLabels: map[string]string{"example.com/unexpected": "true"}},
				},
			}
			Expect(k8sClient.Update(ctx, clusterRole)).To(Succeed())

			clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterClusterRoleBindingName}, clusterRoleBinding)).To(Succeed())
			clusterRoleBinding.Subjects = []rbacv1.Subject{{Kind: "User", Name: "unexpected"}}
			Expect(k8sClient.Update(ctx, clusterRoleBinding)).To(Succeed())

			pullSecretClusterRole := &rbacv1.ClusterRole{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterPullSecretClusterRoleName}, pullSecretClusterRole)).To(Succeed())
			pullSecretClusterRole.Rules = nil
			pullSecretClusterRole.AggregationRule = &rbacv1.AggregationRule{
				ClusterRoleSelectors: []metav1.LabelSelector{
					{MatchLabels: map[string]string{"example.com/unexpected": "true"}},
				},
			}
			Expect(k8sClient.Update(ctx, pullSecretClusterRole)).To(Succeed())

			pullSecretRoleBinding := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
				Namespace: utils.TelemetryPullSecretNamespace,
			}, pullSecretRoleBinding)).To(Succeed())
			pullSecretRoleBinding.Subjects = []rbacv1.Subject{{Kind: "User", Name: "unexpected"}}
			Expect(k8sClient.Update(ctx, pullSecretRoleBinding)).To(Succeed())

			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())

			desiredCM, err := GenerateOtelDataverseExporterConfigMap(testReconcilerInstance, testCR)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			Expect(cm.Data).To(Equal(desiredCM.Data))

			desiredClusterRole, err := GenerateOtelDataverseExporterClusterRole(testReconcilerInstance, testCR)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterClusterRoleName}, clusterRole)).To(Succeed())
			Expect(clusterRole.Rules).To(Equal(desiredClusterRole.Rules))
			Expect(clusterRole.AggregationRule).To(BeNil())

			desiredClusterRoleBinding, err := GenerateOtelDataverseExporterClusterRoleBinding(testReconcilerInstance, testCR)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterClusterRoleBindingName}, clusterRoleBinding)).To(Succeed())
			Expect(clusterRoleBinding.Subjects).To(Equal(desiredClusterRoleBinding.Subjects))

			desiredPullSecretClusterRole, err := GenerateOtelDataverseExporterPullSecretClusterRole(testReconcilerInstance, testCR)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: utils.OtelDataverseExporterPullSecretClusterRoleName}, pullSecretClusterRole)).To(Succeed())
			Expect(pullSecretClusterRole.Rules).To(Equal(desiredPullSecretClusterRole.Rules))
			Expect(pullSecretClusterRole.AggregationRule).To(BeNil())

			desiredPullSecretRoleBinding, err := GenerateOtelDataverseExporterPullSecretRoleBinding(testReconcilerInstance, testCR)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
				Namespace: utils.TelemetryPullSecretNamespace,
			}, pullSecretRoleBinding)).To(Succeed())
			Expect(pullSecretRoleBinding.Subjects).To(Equal(desiredPullSecretRoleBinding.Subjects))
		})

		It("should remove and recreate all exporter resources when transcripts are disabled", func() {
			disabledCR := testCR.DeepCopy()
			disabledCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = true
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, disabledCR)).To(Succeed())
			expectOtelDataverseExporterResourcesAbsent()

			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectOtelDataverseExporterResourcesPresent()
		})

		It("should remove exporter resources when valid telemetry auth is absent or empty", func() {
			setTelemetryPullSecretForTest(telemetryPullSecretWithoutAuthTest, corev1.SecretTypeDockerConfigJson)
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectOtelDataverseExporterResourcesAbsent()

			setTelemetryPullSecretForTest(`{"auths":{"cloud.openshift.com":{"auth":"  "}}}`, corev1.SecretTypeDockerConfigJson)
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectOtelDataverseExporterResourcesAbsent()

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			Expect(cm.Data[utils.OtelCollectorConfigMapDataKey]).To(ContainSubstring("file/data_collection"))

			setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectOtelDataverseExporterResourcesPresent()
		})

		It("should preserve exporter resources while reporting malformed or missing telemetry auth", func() {
			DeferCleanup(func() {
				setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
			})

			setTelemetryPullSecretForTest("not-json", corev1.SecretTypeOpaque)
			err := ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to decode telemetry pull secret"))
			expectOtelDataverseExporterResourcesPresent()

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.TelemetryPullSecretName,
				Namespace: utils.TelemetryPullSecretNamespace,
			}, secret)).To(Succeed())
			secret.Data = map[string][]byte{}
			Expect(k8sClient.Update(ctx, secret)).To(Succeed())

			err = ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not contain " + corev1.DockerConfigJsonKey))
			expectOtelDataverseExporterResourcesPresent()
		})

	})

	Context("Phase 2 deployment", func() {
		BeforeAll(func() {
			ensureServiceCAConfigMap()
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
			Expect(svc.Spec.Ports).To(HaveLen(4))
			Expect(svc.Spec.Ports[3].Name).To(Equal("metrics"))
			Expect(svc.Spec.Ports[3].Port).To(Equal(int32(utils.OtelCollectorMetricsPort)))
		})

		It("should create the collector ServiceMonitor", func() {
			sm := &monv1.ServiceMonitor{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorServiceMonitorName,
				Namespace: utils.OLSNamespaceDefault,
			}, sm)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(sm)
			Expect(sm.Spec.Endpoints).To(HaveLen(1))
			Expect(sm.Spec.Endpoints[0].Port).To(Equal("metrics"))
			Expect(string(*sm.Spec.Endpoints[0].Scheme)).To(Equal("https"))
		})

		It("should trigger a rolling restart via RestartOtelCollector", func() {
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)).To(Succeed())
			oldReload := dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]

			err := RestartOtelCollector(testReconcilerInstance, ctx, dep)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)).To(Succeed())
			Expect(dep.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
			Expect(dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]).NotTo(Equal(oldReload))
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
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(testOtelCollectorImage))
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

		It("should trigger a rolling restart when the exporter ConfigMap changes", func() {
			dep := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)).To(Succeed())
			oldExporterConfigMapVersion := dep.Annotations[utils.OtelDataverseExporterConfigMapResourceVersionAnnotation]
			Expect(oldExporterConfigMapVersion).NotTo(BeEmpty())
			oldForceReload := dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]

			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelDataverseExporterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)).To(Succeed())
			cm.Data[utils.OtelDataverseExporterConfigMapDataKey] += "\n# config refresh"
			Expect(k8sClient.Update(ctx, cm)).To(Succeed())

			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, dep)).To(Succeed())
			Expect(dep.Annotations[utils.OtelDataverseExporterConfigMapResourceVersionAnnotation]).NotTo(Equal(oldExporterConfigMapVersion))
			Expect(dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]).NotTo(Equal(oldForceReload))

			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())
		})

		It("should reconcile telemetry auth and transcript collection against an existing Deployment", func() {
			deploymentKey := types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}
			getDeployment := func() *appsv1.Deployment {
				deployment := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, deploymentKey, deployment)).To(Succeed())
				return deployment
			}

			existingDeployment := getDeployment()
			Expect(existingDeployment.UID).NotTo(BeEmpty())
			existingDeploymentUID := existingDeployment.UID

			readTraceExporterPath := func() (string, bool) {
				configMap := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.OtelCollectorConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				}, configMap)).To(Succeed())

				var collectorConfig struct {
					Exporters map[string]struct {
						Path string `json:"path"`
					} `json:"exporters"`
				}
				Expect(yaml.Unmarshal([]byte(configMap.Data[utils.OtelCollectorConfigMapDataKey]), &collectorConfig)).To(Succeed())
				traceExporter, found := collectorConfig.Exporters["file/data_collection"]
				return traceExporter.Path, found
			}

			expectExporterResourcesPresent := func() {
				expectOtelDataverseExporterResourcesPresent()

				configMap := &corev1.ConfigMap{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      utils.OtelDataverseExporterConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				}, configMap)).To(Succeed())
				var exporterConfig struct {
					LedgerFile string `json:"ledger_file"`
				}
				Expect(yaml.Unmarshal([]byte(configMap.Data[utils.OtelDataverseExporterConfigMapDataKey]), &exporterConfig)).To(Succeed())
				Expect(exporterConfig.LedgerFile).To(Equal(utils.OtelDataverseExporterLedgerFilePath))
			}

			expectExporterDeploymentPresent := func(deployment *appsv1.Deployment) {
				podSpec := deployment.Spec.Template.Spec
				Expect(podSpec.Containers).To(HaveLen(2))
				Expect(podSpec.Containers[0].Name).To(Equal(utils.OtelCollectorContainerName))
				exporter := podSpec.Containers[1]
				Expect(exporter.Name).To(Equal(utils.DataverseExporterContainerName))

				configVolume, found := findVolume(podSpec.Volumes, utils.OtelDataverseExporterConfigVolumeName)
				Expect(found).To(BeTrue())
				Expect(configVolume.ConfigMap).NotTo(BeNil())
				stateVolume, found := findVolume(podSpec.Volumes, utils.OtelDataverseExporterStateVolumeName)
				Expect(found).To(BeTrue())
				Expect(stateVolume.EmptyDir).NotTo(BeNil())
				authVolume, found := findVolume(podSpec.Volumes, utils.OtelDataverseExporterAuthVolumeName)
				Expect(found).To(BeTrue())
				Expect(authVolume.Projected).NotTo(BeNil())

				configMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelDataverseExporterConfigVolumeName)
				Expect(found).To(BeTrue())
				Expect(configMount.MountPath).To(Equal(utils.OtelDataverseExporterConfigMountPath))
				Expect(configMount.ReadOnly).To(BeTrue())
				sourceMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelCollectorDataCollectionVolumeName)
				Expect(found).To(BeTrue())
				Expect(sourceMount.MountPath).To(Equal(utils.OtelDataverseExporterDataMountPath))
				Expect(sourceMount.ReadOnly).To(BeTrue())
				stateMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelDataverseExporterStateVolumeName)
				Expect(found).To(BeTrue())
				Expect(stateMount.MountPath).To(Equal(utils.OtelDataverseExporterStateMountPath))
				Expect(stateMount.ReadOnly).To(BeFalse())
				authMount, found := findVolumeMount(exporter.VolumeMounts, utils.OtelDataverseExporterAuthVolumeName)
				Expect(found).To(BeTrue())
				Expect(authMount.MountPath).To(Equal(utils.OtelDataverseExporterAuthMountPath))
				Expect(authMount.ReadOnly).To(BeTrue())
			}

			expectExporterDeploymentAbsent := func(deployment *appsv1.Deployment) {
				podSpec := deployment.Spec.Template.Spec
				Expect(podSpec.Containers).To(HaveLen(1))
				Expect(podSpec.Containers[0].Name).To(Equal(utils.OtelCollectorContainerName))
				for _, name := range []string{
					utils.OtelDataverseExporterConfigVolumeName,
					utils.OtelDataverseExporterStateVolumeName,
					utils.OtelDataverseExporterAuthVolumeName,
				} {
					_, found := findVolume(podSpec.Volumes, name)
					Expect(found).To(BeFalse())
					_, found = findVolumeMount(podSpec.Containers[0].VolumeMounts, name)
					Expect(found).To(BeFalse())
				}
			}

			expectSourceVolumePresent := func(deployment *appsv1.Deployment) {
				podSpec := deployment.Spec.Template.Spec
				sourceVolume, found := findVolume(podSpec.Volumes, utils.OtelCollectorDataCollectionVolumeName)
				Expect(found).To(BeTrue())
				Expect(sourceVolume.EmptyDir).NotTo(BeNil())
				Expect(sourceVolume.EmptyDir.SizeLimit).NotTo(BeNil())
				Expect(sourceVolume.EmptyDir.SizeLimit.String()).To(Equal(utils.OtelCollectorDataCollectionSizeLimitDefault))

				sourceMount, found := findVolumeMount(podSpec.Containers[0].VolumeMounts, utils.OtelCollectorDataCollectionVolumeName)
				Expect(found).To(BeTrue())
				Expect(sourceMount.MountPath).To(Equal(utils.OtelCollectorDataCollectionMountPath))
				Expect(sourceMount.ReadOnly).To(BeFalse())
			}

			expectSourceVolumeAbsent := func(deployment *appsv1.Deployment) {
				podSpec := deployment.Spec.Template.Spec
				_, found := findVolume(podSpec.Volumes, utils.OtelCollectorDataCollectionVolumeName)
				Expect(found).To(BeFalse())
				_, found = findVolumeMount(podSpec.Containers[0].VolumeMounts, utils.OtelCollectorDataCollectionVolumeName)
				Expect(found).To(BeFalse())
			}

			expectDeploymentRollout := func(previous *appsv1.Deployment) *appsv1.Deployment {
				updated := getDeployment()
				Expect(updated.UID).To(Equal(existingDeploymentUID))
				Expect(updated.ResourceVersion).NotTo(Equal(previous.ResourceVersion))
				Expect(updated.Spec.Template).NotTo(Equal(previous.Spec.Template))
				reload := updated.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]
				Expect(reload).NotTo(BeEmpty())
				Expect(reload).NotTo(Equal(previous.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]))
				return updated
			}

			setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectExporterResourcesPresent()
			sourcePath, sourceConfigured := readTraceExporterPath()
			Expect(sourceConfigured).To(BeTrue())
			Expect(sourcePath).To(Equal(utils.OtelCollectorDataCollectionTraceFilePath))
			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())
			deployment := getDeployment()
			Expect(deployment.UID).To(Equal(existingDeploymentUID))
			expectExporterDeploymentPresent(deployment)
			expectSourceVolumePresent(deployment)

			deploymentBeforeSecretLoss := getDeployment()
			utils.DeleteTelemetryPullSecret(ctx, k8sClient)
			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.TelemetryPullSecretName,
				Namespace: utils.TelemetryPullSecretNamespace,
			}, &corev1.Secret{}))).To(BeTrue())
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectOtelDataverseExporterResourcesAbsent()
			sourcePathAfterSecretLoss, sourceConfiguredAfterSecretLoss := readTraceExporterPath()
			Expect(sourceConfiguredAfterSecretLoss).To(BeTrue())
			Expect(sourcePathAfterSecretLoss).To(Equal(sourcePath))
			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())
			deployment = expectDeploymentRollout(deploymentBeforeSecretLoss)
			expectExporterDeploymentAbsent(deployment)
			expectSourceVolumePresent(deployment)

			deploymentBeforeAuthRestore := getDeployment()
			setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectExporterResourcesPresent()
			sourcePathAfterAuthRestore, sourceConfiguredAfterAuthRestore := readTraceExporterPath()
			Expect(sourceConfiguredAfterAuthRestore).To(BeTrue())
			Expect(sourcePathAfterAuthRestore).To(Equal(sourcePath))
			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())
			deployment = expectDeploymentRollout(deploymentBeforeAuthRestore)
			expectExporterDeploymentPresent(deployment)
			expectSourceVolumePresent(deployment)

			transcriptsDisabledCR := testCR.DeepCopy()
			transcriptsDisabledCR.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled = true
			deploymentBeforeTranscriptOptOut := getDeployment()
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, transcriptsDisabledCR)).To(Succeed())
			expectOtelDataverseExporterResourcesAbsent()
			_, sourceConfiguredAfterTranscriptOptOut := readTraceExporterPath()
			Expect(sourceConfiguredAfterTranscriptOptOut).To(BeFalse())
			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, transcriptsDisabledCR)).To(Succeed())
			deployment = expectDeploymentRollout(deploymentBeforeTranscriptOptOut)
			expectExporterDeploymentAbsent(deployment)
			expectSourceVolumeAbsent(deployment)

			deploymentBeforeRestore := getDeployment()
			setTelemetryPullSecretForTest(telemetryPullSecretWithAuthForTest, corev1.SecretTypeDockerConfigJson)
			Expect(ReconcileOtelCollectorResources(testReconcilerInstance, ctx, testCR)).To(Succeed())
			expectExporterResourcesPresent()
			sourcePathAfterRestore, sourceConfiguredAfterRestore := readTraceExporterPath()
			Expect(sourceConfiguredAfterRestore).To(BeTrue())
			Expect(sourcePathAfterRestore).To(Equal(sourcePath))
			Expect(ReconcileOtelCollectorDeployment(testReconcilerInstance, ctx, testCR)).To(Succeed())
			deployment = expectDeploymentRollout(deploymentBeforeRestore)
			expectExporterDeploymentPresent(deployment)
			expectSourceVolumePresent(deployment)
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

		It("should persist a changed data-collection volume sizeLimit without a ConfigMap change", func() {
			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, deployment)).To(Succeed())

			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, configMap)).To(Succeed())
			configMapResourceVersion := configMap.ResourceVersion
			Expect(configMapResourceVersion).NotTo(BeEmpty())
			Expect(deployment.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation]).To(Equal(configMapResourceVersion))

			findDataCollectionVolume := func(volumes []corev1.Volume) *corev1.Volume {
				for i := range volumes {
					if volumes[i].Name == utils.OtelCollectorDataCollectionVolumeName {
						return &volumes[i]
					}
				}
				return nil
			}

			currentVolume := findDataCollectionVolume(deployment.Spec.Template.Spec.Volumes)
			Expect(currentVolume).NotTo(BeNil())
			Expect(currentVolume.EmptyDir).NotTo(BeNil())
			Expect(currentVolume.EmptyDir.SizeLimit).NotTo(BeNil())
			Expect(currentVolume.EmptyDir.SizeLimit.String()).To(Equal("500Mi"))

			oldForceReload := deployment.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]
			desiredDeployment := deployment.DeepCopy()
			desiredVolume := findDataCollectionVolume(desiredDeployment.Spec.Template.Spec.Volumes)
			Expect(desiredVolume).NotTo(BeNil())
			desiredSizeLimit := resource.MustParse("600Mi")
			desiredVolume.EmptyDir.SizeLimit = &desiredSizeLimit

			Expect(UpdateOtelCollectorDeployment(testReconcilerInstance, ctx, deployment, desiredDeployment)).To(Succeed())

			updatedDeployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, updatedDeployment)).To(Succeed())
			updatedVolume := findDataCollectionVolume(updatedDeployment.Spec.Template.Spec.Volumes)
			Expect(updatedVolume).NotTo(BeNil())
			Expect(updatedVolume.EmptyDir).NotTo(BeNil())
			Expect(updatedVolume.EmptyDir.SizeLimit).NotTo(BeNil())
			Expect(updatedVolume.EmptyDir.SizeLimit.String()).To(Equal("600Mi"))

			updatedConfigMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.OtelCollectorConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, updatedConfigMap)).To(Succeed())
			Expect(updatedConfigMap.ResourceVersion).To(Equal(configMapResourceVersion))
			Expect(updatedDeployment.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation]).To(Equal(configMapResourceVersion))

			forceReload := updatedDeployment.Spec.Template.Annotations[utils.ForceReloadAnnotationKey]
			Expect(forceReload).NotTo(BeEmpty())
			Expect(forceReload).NotTo(Equal(oldForceReload))
		})
	})
})
