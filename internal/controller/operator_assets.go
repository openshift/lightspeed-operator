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

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
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
	var schemeHTTPS monv1.Scheme = "https"
	serviceMonitor := monv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OperatorServiceMonitorName,
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

func (r *OLSConfigReconciler) ReconcileServiceMonitorForOperator(ctx context.Context) error {
	if !r.Options.PrometheusAvailable {
		r.Logger.Info("Prometheus Operator not available, skipping operator ServiceMonitor reconciliation")
		return nil
	}

	sm, err := r.generateServiceMonitorForOperator()
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateServiceMonitor, err)
	}
	operatorDeployment := &appsv1.Deployment{}
	foundSm := &monv1.ServiceMonitor{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OperatorServiceMonitorName, Namespace: r.Options.Namespace}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		r.Logger.Info("creating a new service monitor", "serviceMonitor", sm.Name)
		err = r.Get(ctx, client.ObjectKey{Name: utils.OperatorDeploymentName, Namespace: r.Options.Namespace}, operatorDeployment)
		if err != nil {
			r.Logger.Error(err, "cannot get operator deployment", "name", utils.OperatorDeploymentName, "namespace", r.Options.Namespace)
			return fmt.Errorf("%s: %w", utils.ErrCreateServiceMonitor, err)
		}
		err = controllerutil.SetOwnerReference(operatorDeployment, sm, r.Scheme())
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateServiceMonitor, err)
		}
		err = r.Create(ctx, sm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateServiceMonitor, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetServiceMonitor, err)
	}
	if utils.ServiceMonitorEqual(foundSm, sm) {
		r.Logger.Info("Lightspeed Operator service monitor unchanged, reconciliation skipped", "serviceMonitor", sm.Name)
		return nil
	}
	foundSm.Spec = sm.Spec
	err = r.Get(ctx, client.ObjectKey{Name: utils.OperatorDeploymentName, Namespace: r.Options.Namespace}, operatorDeployment)
	if err != nil {
		r.Logger.Error(err, "cannot get operator deployment", "name", utils.OperatorDeploymentName, "namespace", r.Options.Namespace)
		return fmt.Errorf("%s: %w", utils.ErrUpdateServiceMonitor, err)
	}
	err = controllerutil.SetOwnerReference(operatorDeployment, sm, r.Scheme())
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateServiceMonitor, err)
	}
	err = r.Update(ctx, foundSm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateServiceMonitor, err)
	}
	r.Logger.Info("Lightspeed Operator service monitor reconciled", "serviceMonitor", sm.Name)
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
			Name:      utils.OperatorNetworkPolicyName,
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
									"kubernetes.io/metadata.name": utils.ClientCACmNamespace,
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
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.OperatorMetricsPort)}[0],
						},
					},
				},
			},
		},
	}

	return np, nil
}

func (r *OLSConfigReconciler) ReconcileNetworkPolicyForOperator(ctx context.Context) error {
	np, err := r.generateNetworkPolicyForOperator()
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOperatorNetworkPolicy, err)
	}
	foundNp := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OperatorNetworkPolicyName, Namespace: r.Options.Namespace}, foundNp)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, &np)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOperatorNetworkPolicy, err)
		}
		r.Logger.Info("created a new network policy", "networkPolicy", np.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOperatorNetworkPolicy, err)
	}
	if utils.NetworkPolicyEqual(foundNp, &np) {
		r.Logger.Info("Operator network policy unchanged, reconciliation skipped", "networkPolicy", np.Name)
		return nil
	}
	foundNp.Spec = np.Spec
	err = r.Update(ctx, foundNp)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOperatorNetworkPolicy, err)
	}
	r.Logger.Info("Operator network policy reconciled", "networkPolicy", np.Name)
	return nil
}
