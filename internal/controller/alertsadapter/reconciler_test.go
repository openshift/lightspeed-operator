package alertsadapter

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
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
	Expect(ownerRef.UID).To(Equal(olsConfig.UID))
}

var _ = Describe("Alerts adapter reconciler", Ordered, func() {
	It("does not create resources when configMapRef is unset", func() {
		err := ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, cr)
		Expect(err).NotTo(HaveOccurred())

		sa := &corev1.ServiceAccount{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      utils.AlertsAdapterServiceAccountName,
			Namespace: utils.OLSNamespaceDefault,
		}, sa)
		Expect(err).To(HaveOccurred())
	})

	It("RemoveAlertsAdapter is idempotent when operand resources are absent", func() {
		err := RemoveAlertsAdapter(testReconcilerInstance, ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	It("retains the egress policy until adapter pods are gone", func() {
		np, err := GenerateEgressNetworkPolicy(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Create(ctx, np)).To(Succeed())

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alerts-adapter-terminating",
				Namespace: utils.OLSNamespaceDefault,
				Labels:    utils.GenerateAlertsAdapterSelectorLabels(),
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: utils.AlertsAdapterContainerName, Image: "test"}},
			},
		}
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())

		err = RemoveAlertsAdapter(testReconcilerInstance, ctx)
		Expect(errors.Is(err, ErrAlertsAdapterCleanupPending)).To(BeTrue())

		found := &networkingv1.NetworkPolicy{}
		key := types.NamespacedName{Name: utils.AlertsAdapterEgressNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}
		Expect(k8sClient.Get(ctx, key, found)).To(Succeed())

		Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
		Expect(RemoveAlertsAdapter(testReconcilerInstance, ctx)).To(Succeed())
		err = k8sClient.Get(ctx, key, found)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	Context("ConfigMap without operator validation", func() {
		It("reconciles Phase 1 when the referenced ConfigMap exists without config.yaml", func() {
			enabledCR := cr.DeepCopy()
			enabledCR.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.ConfigMapRef = &corev1.LocalObjectReference{
				Name: utils.AlertsAdapterConfigMapName,
			}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.AlertsAdapterConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{"other.yaml": "pollInterval: 30s\n"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}()

			err := ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, enabledCR)
			Expect(err).NotTo(HaveOccurred())
		})

		It("reconciles Phase 2 deployment when the referenced ConfigMap has no config.yaml", func() {
			enabledCR := cr.DeepCopy()
			enabledCR.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.ConfigMapRef = &corev1.LocalObjectReference{
				Name: utils.AlertsAdapterConfigMapName,
			}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.AlertsAdapterConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{"other.yaml": "pollInterval: 30s\n"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer func() {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}()

			err := ReconcileAlertsAdapterDeployment(testReconcilerInstance, ctx, enabledCR)
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
		})
	})

	Context("Creation logic", Ordered, func() {
		var enabledCR *olsv1alpha1.OLSConfig

		BeforeAll(func() {
			enabledCR = cr.DeepCopy()
			enabledCR.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.ConfigMapRef = &corev1.LocalObjectReference{
				Name: utils.AlertsAdapterConfigMapName,
			}

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.AlertsAdapterConfigMapName,
					Namespace: utils.OLSNamespaceDefault,
				},
				Data: map[string]string{
					utils.AlertsAdapterConfigMapDataKey: "pollInterval: 30s\ninitialDelay: 5m\ncooldownWindow: 1h\n",
				},
			}
			err := k8sClient.Create(ctx, cm)
			Expect(client.IgnoreAlreadyExists(err)).NotTo(HaveOccurred())

			err = ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, enabledCR)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			Expect(RemoveAlertsAdapter(testReconcilerInstance, ctx)).To(Succeed())

			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			}, cm)
			if err == nil {
				Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
			}
		})

		It("should create the service account", func() {
			sa := &corev1.ServiceAccount{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}, sa)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(sa)
		})

		It("should create the agenticruns Role and RoleBinding", func() {
			role := &rbacv1.Role{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterAgenticRunsRoleName,
				Namespace: utils.OLSNamespaceDefault,
			}, role)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(role)

			rb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterAgenticRunsRoleBindingName,
				Namespace: utils.OLSNamespaceDefault,
			}, rb)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(rb)
		})

		It("should create the agenticolsconfig ClusterRole and ClusterRoleBinding", func() {
			role := &rbacv1.ClusterRole{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.AlertsAdapterAgenticOLSConfigClusterRoleName,
			}, role)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(role)

			rb := &rbacv1.ClusterRoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.AlertsAdapterAgenticOLSConfigClusterRoleBindingName,
			}, rb)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(rb)
		})

		It("restores agenticolsconfig ClusterRole rule drift without updating an unchanged role", func() {
			role := &rbacv1.ClusterRole{}
			key := types.NamespacedName{Name: utils.AlertsAdapterAgenticOLSConfigClusterRoleName}
			Expect(k8sClient.Get(ctx, key, role)).To(Succeed())
			Expect(role.Rules).To(HaveLen(1))

			role.Rules[0].ResourceNames = nil
			Expect(k8sClient.Update(ctx, role)).To(Succeed())
			Expect(ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, enabledCR)).To(Succeed())

			Expect(k8sClient.Get(ctx, key, role)).To(Succeed())
			desiredRole, err := GenerateAgenticOLSConfigClusterRole(testReconcilerInstance, enabledCR)
			Expect(err).NotTo(HaveOccurred())
			Expect(role.Rules).To(Equal(desiredRole.Rules))

			version := role.ResourceVersion
			Expect(ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, enabledCR)).To(Succeed())
			Expect(k8sClient.Get(ctx, key, role)).To(Succeed())
			Expect(role.ResourceVersion).To(Equal(version))
		})

		It("should create the Alertmanager RoleBinding", func() {
			rb := &rbacv1.RoleBinding{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterAlertmanagerRoleBindingName,
				Namespace: utils.OpenShiftMonitoringNamespace,
			}, rb)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the network policy", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the egress network policy", func() {
			np := &networkingv1.NetworkPolicy{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterEgressNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, np)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(np)
		})

		It("should reconcile drifted egress policy spec", func() {
			found := &networkingv1.NetworkPolicy{}
			key := types.NamespacedName{Name: utils.AlertsAdapterEgressNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}
			Expect(k8sClient.Get(ctx, key, found)).To(Succeed())
			found.Spec.Egress = nil
			Expect(k8sClient.Update(ctx, found)).To(Succeed())

			Expect(ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, enabledCR)).To(Succeed())

			desired, err := GenerateEgressNetworkPolicy(testReconcilerInstance, enabledCR)
			Expect(err).NotTo(HaveOccurred())
			reconciled := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, key, reconciled)).To(Succeed())
			Expect(utils.NetworkPolicyEqual(desired, reconciled)).To(BeTrue())
		})

		It("should not create legacy config Role or RoleBinding", func() {
			role := &rbacv1.Role{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterConfigRoleName,
				Namespace: utils.OLSNamespaceDefault,
			}, role)
			Expect(err).To(HaveOccurred())

			rb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterConfigRoleBindingName,
				Namespace: utils.OLSNamespaceDefault,
			}, rb)
			Expect(err).To(HaveOccurred())
		})

		It("should create the deployment", func() {
			err := ReconcileAlertsAdapterDeployment(testReconcilerInstance, ctx, enabledCR)
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())
			expectOwnedByOLSConfig(deployment)
			Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
				Name:  utils.AlertsAdapterAlertmanagerURLEnvVar,
				Value: utils.AlertsAdapterAlertmanagerURL,
			}))
			Expect(deployment.Spec.Template.Spec.Containers[0].SecurityContext).NotTo(BeNil())
			Expect(*deployment.Spec.Template.Spec.Containers[0].SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(deployment.Spec.Template.Spec.Volumes[1].ConfigMap.Name).To(Equal(utils.AlertsAdapterConfigMapName))
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
				Name:      utils.AlertsAdapterConfigVolumeName,
				MountPath: utils.AlertsAdapterConfigVolumeMountPath,
				ReadOnly:  true,
			}))
		})

		It("should trigger a rolling restart via ForceReload annotation", func() {
			deployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			err = RestartAlertsAdapter(testReconcilerInstance, ctx, deployment)
			Expect(err).NotTo(HaveOccurred())

			updated := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Spec.Template.Annotations).To(HaveKey(utils.ForceReloadAnnotationKey))
		})

		It("should remove all operand resources when configMapRef is unset", func() {
			egressPolicy := &networkingv1.NetworkPolicy{}
			egressPolicyKey := types.NamespacedName{Name: utils.AlertsAdapterEgressNetworkPolicyName, Namespace: utils.OLSNamespaceDefault}
			err := k8sClient.Get(ctx, egressPolicyKey, egressPolicy)
			if apierrors.IsNotFound(err) {
				egressPolicy, err = GenerateEgressNetworkPolicy(testReconcilerInstance, enabledCR)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Create(ctx, egressPolicy)).To(Succeed())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}

			disabledCR := cr.DeepCopy()
			err = ReconcileAlertsAdapterResources(testReconcilerInstance, ctx, disabledCR)
			Expect(err).NotTo(HaveOccurred())

			removedEgressPolicy := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, egressPolicyKey, removedEgressPolicy)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			removedIngressPolicy := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterNetworkPolicyName,
				Namespace: utils.OLSNamespaceDefault,
			}, removedIngressPolicy)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterDeploymentName,
				Namespace: utils.OLSNamespaceDefault,
			}, deployment)
			Expect(err).To(HaveOccurred())

			sa := &corev1.ServiceAccount{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterServiceAccountName,
				Namespace: utils.OLSNamespaceDefault,
			}, sa)
			Expect(err).To(HaveOccurred())

			role := &rbacv1.Role{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      utils.AlertsAdapterAgenticRunsRoleName,
				Namespace: utils.OLSNamespaceDefault,
			}, role)
			Expect(err).To(HaveOccurred())

			clusterRole := &rbacv1.ClusterRole{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.AlertsAdapterAgenticOLSConfigClusterRoleName,
			}, clusterRole)
			Expect(err).To(HaveOccurred())

			clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: utils.AlertsAdapterAgenticOLSConfigClusterRoleBindingName,
			}, clusterRoleBinding)
			Expect(err).To(HaveOccurred())
		})
	})
})
