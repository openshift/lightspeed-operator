package alertsadapter

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

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

	It("should generate the agenticruns Role", func() {
		role, err := GenerateAgenticRunsRole(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(role.Name).To(Equal(utils.AlertsAdapterAgenticRunsRoleName))
		Expect(role.Namespace).To(Equal(utils.OLSNamespaceDefault))
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

	It("should keep the existing ingress-deny policy ingress-only", func() {
		np, err := GenerateNetworkPolicy(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress}))
		Expect(np.Spec.Ingress).To(BeEmpty())
		Expect(np.Spec.Egress).To(BeEmpty())
	})

	It("should generate an egress-only policy for the API, platform Alertmanager, and DNS", func() {
		np, err := GenerateEgressNetworkPolicy(testReconcilerInstance, cr)
		Expect(err).NotTo(HaveOccurred())

		adapterLabels := map[string]string{
			"app":                          "lightspeed-agentic-alerts-adapter",
			"app.kubernetes.io/component":  "alerts-adapter",
			"app.kubernetes.io/managed-by": "lightspeed-operator",
			"app.kubernetes.io/name":       "lightspeed-agentic-alerts-adapter",
			"app.kubernetes.io/part-of":    "openshift-lightspeed",
		}
		Expect(np.Name).To(Equal("lightspeed-agentic-alerts-adapter-egress"))
		Expect(np.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(np.Labels).To(Equal(adapterLabels))
		Expect(np.Spec.PodSelector.MatchLabels).To(Equal(adapterLabels))
		Expect(np.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeEgress}))
		Expect(np.Spec.Ingress).To(BeEmpty())
		Expect(np.Spec.Egress).To(HaveLen(3))

		tcp := corev1.ProtocolTCP
		udp := corev1.ProtocolUDP
		apiPort := intstr.FromInt32(6443)
		alertmanagerPort := intstr.FromInt32(9095)
		dnsPodPort := intstr.FromInt32(5353)
		Expect(np.Spec.Egress).To(ContainElement(networkingv1.NetworkPolicyEgressRule{
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &apiPort}},
		}))
		Expect(np.Spec.Egress).To(ContainElement(networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": "openshift-monitoring",
				}},
				PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app.kubernetes.io/name":     "alertmanager",
					"app.kubernetes.io/instance": "main",
					"app.kubernetes.io/part-of":  "openshift-monitoring",
				}},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &alertmanagerPort}},
		}))
		Expect(np.Spec.Egress).To(ContainElement(networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": "openshift-dns",
				}},
				PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"dns.operator.openshift.io/daemonset-dns": "default",
				}},
			}},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &udp, Port: &dnsPodPort},
				{Protocol: &tcp, Port: &dnsPodPort},
			},
		}))

		Expect(np.OwnerReferences).To(HaveLen(1))
		Expect(np.OwnerReferences[0].APIVersion).To(Equal(utils.OLSConfigAPIVersion))
		Expect(np.OwnerReferences[0].Kind).To(Equal(utils.OLSConfigKind))
		Expect(np.OwnerReferences[0].Name).To(Equal(cr.Name))
	})

	It("should generate the deployment without a config volume when configMapRef is unset", func() {
		deployment, err := GenerateDeployment(testReconcilerInstance, ctx, cr)
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment.Name).To(Equal(utils.AlertsAdapterDeploymentName))
		Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal(utils.AlertsAdapterServiceAccountName))
		Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
		Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.AlertsAdapterContainerName))
		Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(testReconcilerInstance.GetAlertsAdapterImage()))
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
