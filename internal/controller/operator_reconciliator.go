package controller

import (
	"context"
	"fmt"
	"strings"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
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
	operatorDeployment := &appsv1.Deployment{}
	foundSm := &monv1.ServiceMonitor{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OperatorServiceMonitorName, Namespace: r.Options.Namespace}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new service monitor", "serviceMonitor", sm.Name)
		err = r.Client.Get(ctx, client.ObjectKey{Name: OperatorDeploymentName, Namespace: r.Options.Namespace}, operatorDeployment)
		if err != nil {
			r.logger.Error(err, "cannot get operator deployment", "name", OperatorDeploymentName, "namespace", r.Options.Namespace)
			return fmt.Errorf("%s: %w", ErrCreateServiceMonitor, err)
		}
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
	err = r.Client.Get(ctx, client.ObjectKey{Name: OperatorDeploymentName, Namespace: r.Options.Namespace}, operatorDeployment)
	if err != nil {
		r.logger.Error(err, "cannot get operator deployment", "name", OperatorDeploymentName, "namespace", r.Options.Namespace)
		return fmt.Errorf("%s: %w", ErrUpdateServiceMonitor, err)
	}
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
