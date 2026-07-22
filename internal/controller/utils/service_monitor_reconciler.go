package utils

import (
	"context"
	"fmt"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

// ReconcileServiceMonitor creates or updates a ServiceMonitor when Prometheus Operator
// CRDs are available. Callers generate the desired object (including owner references).
// Skips silently when Prometheus is not available.
func ReconcileServiceMonitor(r reconciler.Reconciler, ctx context.Context, desired *monv1.ServiceMonitor) error {
	if !r.IsPrometheusAvailable() {
		r.GetLogger().Info("Prometheus Operator not available, skipping ServiceMonitor reconciliation",
			"serviceMonitor", desired.Name)
		return nil
	}

	found := &monv1.ServiceMonitor{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: r.GetNamespace()}, found)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating ServiceMonitor", "serviceMonitor", desired.Name)
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateServiceMonitor, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetServiceMonitor, err)
	}

	if ServiceMonitorEqual(found, desired) {
		r.GetLogger().Info("ServiceMonitor unchanged, reconciliation skipped", "serviceMonitor", desired.Name)
		return nil
	}

	found.Spec = desired.Spec
	found.Labels = desired.Labels
	if err := r.Update(ctx, found); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateServiceMonitor, err)
	}
	r.GetLogger().Info("ServiceMonitor reconciled", "serviceMonitor", desired.Name)
	return nil
}
