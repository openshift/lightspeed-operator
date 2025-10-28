package controller

import (
	"context"
	"fmt"
	"strings"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *OLSConfigReconciler) generateServiceMonitorForOperator() (*monv1.ServiceMonitor, error) {
	metaLabels := map[string]string{
		"control-plane":                              "controller-manager",
		"app.kubernetes.io/component":                "metrics",
		"app.kubernetes.io/managed-by":               "lightspeed-operator",
		"app.kubernetes.io/name":                     "servicemonitor",
		"app.kubernetes.io/instance":                 "controller-manager-metrics-monitor",
		"app.kubernetes.io/part-of":                  "lightspeed-operator",
		"monitoring.openshift.io/collection-profile": "full",
		"openshift.io/user-monitoring":               "false",
	}

	valFalse := false
	serverName := strings.Join([]string{"lightspeed-operator-controller-manager-service", r.Options.Namespace, "svc"}, ".")
	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OperatorServiceMonitorName,
			Namespace: r.Options.Namespace,
			Labels:    metaLabels,
		},
		Spec: monv1.ServiceMonitorSpec{
			Endpoints: []monv1.Endpoint{
				{
					Port:     "metrics",
					Path:     "/metrics",
					Interval: "30s",
					Scheme:   "https",
					TLSConfig: &monv1.TLSConfig{
						CAFile:   "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
						CertFile: "/etc/prometheus/secrets/metrics-client-certs/tls.crt",
						KeyFile:  "/etc/prometheus/secrets/metrics-client-certs/tls.key",
						SafeTLSConfig: monv1.SafeTLSConfig{
							InsecureSkipVerify: &valFalse,
							ServerName:         &serverName,
						},
					},
				},
			},
			JobLabel: "app.kubernetes.io/name",
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "controller-manager",
				},
			},
		},
	}

	return &serviceMonitor, nil
}

func (r *OLSConfigReconciler) reconcileServiceMonitorForOperator(ctx context.Context) error {
	sm, err := r.generateServiceMonitorForOperator()
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateServiceMonitor, err)
	}

	// Check if operator deployment exists (won't exist when running locally)
	operatorDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: OperatorDeploymentName, Namespace: r.Options.Namespace}, operatorDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("Operator deployment not found, skipping ServiceMonitor creation (likely running locally)",
			"name", OperatorDeploymentName, "namespace", r.Options.Namespace)
		return nil
	} else if err != nil {
		r.logger.Error(err, "error checking operator deployment", "name", OperatorDeploymentName, "namespace", r.Options.Namespace)
		return fmt.Errorf("failed to check operator deployment: %w", err)
	}

	foundSm := &monv1.ServiceMonitor{}
	err = r.Get(ctx, client.ObjectKey{Name: OperatorServiceMonitorName, Namespace: r.Options.Namespace}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service monitor", "serviceMonitor", sm.Name)
		err = controllerutil.SetOwnerReference(operatorDeployment, sm, r.Scheme)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreateServiceMonitor, err)
		}
		err = r.Create(ctx, sm)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreateServiceMonitor, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetServiceMonitor, err)
	}
	if serviceMonitorEqual(foundSm, sm) {
		r.logger.Info("Lightspeed Operator service monitor unchanged, reconciliation skipped", "serviceMonitor", sm.Name)
		return nil
	}
	foundSm.Spec = sm.Spec
	err = controllerutil.SetOwnerReference(operatorDeployment, sm, r.Scheme)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateServiceMonitor, err)
	}
	err = r.Update(ctx, foundSm)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateServiceMonitor, err)
	}
	r.logger.Info("Lightspeed Operator service monitor reconciled", "serviceMonitor", sm.Name)
	return nil
}

func (r *OLSConfigReconciler) generateNetworkPolicyForOperator() (networkingv1.NetworkPolicy, error) {
	metaLabels := map[string]string{
		"app.kubernetes.io/component":  "manager",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "networkpolicy",
		"app.kubernetes.io/instance":   "lightspeed-operator",
		"app.kubernetes.io/part-of":    "lightspeed-operator",
	}
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OperatorNetworkPolicyName,
			Namespace: r.Options.Namespace,
			Labels:    metaLabels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "controller-manager",
				},
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-monitoring",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      "app.kubernetes.io/name",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"prometheus"},
									},
									{
										Key:      "prometheus",
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{"k8s"},
									},
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(OperatorMetricsPort)}[0],
						},
					},
				},
			},
		},
	}

	return np, nil
}

func (r *OLSConfigReconciler) reconcileNetworkPolicyForOperator(ctx context.Context) error {
	np, err := r.generateNetworkPolicyForOperator()
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateOperatorNetworkPolicy, err)
	}
	foundNp := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: OperatorNetworkPolicyName, Namespace: r.Options.Namespace}, foundNp)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, &np)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreateOperatorNetworkPolicy, err)
		}
		r.logger.Info("created a new network policy", "networkPolicy", np.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetOperatorNetworkPolicy, err)
	}
	if networkPolicyEqual(foundNp, &np) {
		r.logger.Info("Operator network policy unchanged, reconciliation skipped", "networkPolicy", np.Name)
		return nil
	}
	foundNp.Spec = np.Spec
	err = r.Update(ctx, foundNp)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateOperatorNetworkPolicy, err)
	}
	r.logger.Info("Operator network policy reconciled", "networkPolicy", np.Name)
	return nil
}
