package alertsadapter

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// GenerateServiceAccount generates the alerts adapter ServiceAccount.
func GenerateServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	return utils.GenerateServiceAccount(r, cr, utils.AlertsAdapterServiceAccountName)
}

// GenerateProposalsClusterRole generates the ClusterRole granting proposal create/list/get access.
func GenerateProposalsClusterRole(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRole, error) {
	role := rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.AlertsAdapterProposalsClusterRoleName,
			Labels: utils.GenerateAlertsAdapterSelectorLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"agentic.openshift.io"},
				Resources: []string{"proposals"},
				Verbs:     []string{"create", "list", "get"},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &role, r.GetScheme()); err != nil {
		return nil, err
	}

	return &role, nil
}

// GenerateProposalsClusterRoleBinding binds the alerts adapter ServiceAccount to the proposals ClusterRole.
func GenerateProposalsClusterRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.ClusterRoleBinding, error) {
	rb := rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.AlertsAdapterProposalsClusterRoleBindingName,
			Labels: utils.GenerateAlertsAdapterSelectorLabels(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.AlertsAdapterServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     utils.AlertsAdapterProposalsClusterRoleName,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rb, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rb, nil
}

// GenerateAlertmanagerRoleBinding grants the adapter view access to Alertmanager in openshift-monitoring.
func GenerateAlertmanagerRoleBinding(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*rbacv1.RoleBinding, error) {
	rb := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AlertsAdapterAlertmanagerRoleBindingName,
			Namespace: utils.OpenShiftMonitoringNamespace,
			Labels:    utils.GenerateAlertsAdapterSelectorLabels(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      utils.AlertsAdapterServiceAccountName,
				Namespace: r.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     utils.MonitoringAlertmanagerViewRoleName,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &rb, r.GetScheme()); err != nil {
		return nil, err
	}

	return &rb, nil
}

// GenerateNetworkPolicy generates the NetworkPolicy restricting adapter egress and ingress.
func GenerateNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AlertsAdapterNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAlertsAdapterSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GenerateAlertsAdapterSelectorLabels(),
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
			Egress:  []networkingv1.NetworkPolicyEgressRule{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, err
	}

	return &np, nil
}

// getUserConfigMap loads the referenced ConfigMap when alerts adapter is enabled.
// Returns (nil, nil) when the ConfigMap does not exist.
func getUserConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	name, ok := utils.AlertsAdapterConfigMapRef(cr)
	if !ok {
		return nil, fmt.Errorf("%s: alerts adapter configMapRef is not set", utils.ErrGetAlertsAdapterConfigMap)
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterConfigMap, err)
	}

	return cm, nil
}
