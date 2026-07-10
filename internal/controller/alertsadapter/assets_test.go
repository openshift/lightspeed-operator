package alertsadapter

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func crWithAlertsAdapterConfigMapRef() *olsv1alpha1.OLSConfig {
	crWithRef := cr.DeepCopy()
	crWithRef.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.ConfigMapRef = &corev1.LocalObjectReference{
		Name: utils.AlertsAdapterConfigMapName,
	}
	return crWithRef
}

var _ = Describe("Alerts adapter assets", func() {
	It("should generate the service account", func() {
		sa, err := GenerateServiceAccount(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(sa.Name).To(Equal(utils.AlertsAdapterServiceAccountName))
		Expect(sa.Namespace).To(Equal(utils.OLSNamespaceDefault))
	})

	It("should generate the agenticruns ClusterRole", func() {
		role, err := GenerateAgenticRunsClusterRole(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(role.Name).To(Equal(utils.AlertsAdapterAgenticRunsClusterRoleName))
		Expect(role.Rules).To(ContainElement(rbacv1.PolicyRule{
			APIGroups: []string{"agentic.openshift.io"},
			Resources: []string{"agenticruns"},
			Verbs:     []string{"create", "list", "get"},
		}))
	})

	It("should generate the Alertmanager RoleBinding in openshift-monitoring", func() {
		rb, err := GenerateAlertmanagerRoleBinding(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(rb.Name).To(Equal(utils.AlertsAdapterAlertmanagerRoleBindingName))
		Expect(rb.Namespace).To(Equal(utils.OpenShiftMonitoringNamespace))
		Expect(rb.RoleRef.Name).To(Equal(utils.MonitoringAlertmanagerViewRoleName))
	})

	It("should generate the deployment without a config volume when configMapRef is unset", func() {
		deployment, err := GenerateDeployment(testReconcilerInstance, ctx, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment.Name).To(Equal(utils.AlertsAdapterDeploymentName))
		Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal(utils.AlertsAdapterServiceAccountName))
		Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
		Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.AlertsAdapterContainerName))
		Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.AlertsAdapterImageDefault))
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal(utils.TmpVolumeName))
	})

	It("should mount the referenced ConfigMap when it exists", func() {
		crWithRef := crWithAlertsAdapterConfigMapRef()
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.AlertsAdapterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			},
			Data: map[string]string{
				utils.AlertsAdapterConfigMapDataKey: "pollInterval: 30s\n",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		}()

		deployment, err := GenerateDeployment(testReconcilerInstance, ctx, crWithRef)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
		Expect(deployment.Spec.Template.Spec.Volumes[1].ConfigMap.Name).To(Equal(utils.AlertsAdapterConfigMapName))
		Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
			Name:      utils.AlertsAdapterConfigVolumeName,
			MountPath: utils.AlertsAdapterConfigVolumeMountPath,
			ReadOnly:  true,
		}))
	})

	It("should not mount a config volume when configMapRef is set but the ConfigMap is missing", func() {
		const missingConfigName = "missing-alerts-adapter-config"
		crWithRef := cr.DeepCopy()
		crWithRef.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.ConfigMapRef = &corev1.LocalObjectReference{
			Name: missingConfigName,
		}

		deployment, err := GenerateDeployment(testReconcilerInstance, ctx, crWithRef)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
		Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal(utils.TmpVolumeName))
	})

	It("should mount the referenced ConfigMap even when config.yaml is absent", func() {
		crWithRef := crWithAlertsAdapterConfigMapRef()
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      utils.AlertsAdapterConfigMapName,
				Namespace: utils.OLSNamespaceDefault,
			},
			Data: map[string]string{
				"other.yaml": "pollInterval: 30s\n",
			},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		defer func() {
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		}()

		deployment, err := GenerateDeployment(testReconcilerInstance, ctx, crWithRef)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(2))
		Expect(deployment.Spec.Template.Spec.Volumes[1].ConfigMap.Name).To(Equal(utils.AlertsAdapterConfigMapName))
	})

	It("does not enable the adapter when configMapRef is unset", func() {
		_, ok := utils.AlertsAdapterConfigMapRef(cr)
		Expect(ok).To(BeFalse())
	})

	It("enables the adapter when configMapRef is set", func() {
		name, ok := utils.AlertsAdapterConfigMapRef(crWithAlertsAdapterConfigMapRef())
		Expect(ok).To(BeTrue())
		Expect(name).To(Equal(utils.AlertsAdapterConfigMapName))
	})
})
