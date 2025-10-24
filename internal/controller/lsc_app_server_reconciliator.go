package controller

import (
	"context"
	"fmt"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func (r *OLSConfigReconciler) reconcileAppServerLSC(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcileAppServerLSC starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile ServiceAccount",
			Task: r.reconcileServiceAccount,
		},
		{
			Name: "reconcile SARRole",
			Task: r.reconcileSARRole,
		},
		{
			Name: "reconcile SARRoleBinding",
			Task: r.reconcileSARRoleBinding,
		},
		// todo: LSC config map generation
		// todo: Llama Stack configmap generation
		{
			Name: "reconcile OLSConfigMap",
			Task: r.reconcileLSCConfigMap,
		},
		{
			Name: "reconcile Additional CA ConfigMap",
			Task: r.reconcileOLSAdditionalCAConfigMap,
		},
		{
			Name: "reconcile App Service",
			Task: r.reconcileService,
		},
		{
			Name: "reconcile App TLS Certs",
			Task: r.reconcileTLSSecret,
		},
		// todo: LSC deployment generation
		{
			Name: "reconcile App Deployment",
			Task: r.reconcileLSCDeployment,
		},
		{
			Name: "reconcile Metrics Reader Secret",
			Task: r.reconcileMetricsReaderSecret,
		},
		{
			Name: "reconcile App ServiceMonitor",
			Task: r.reconcileServiceMonitor,
		},
		{
			Name: "reconcile App PrometheusRule",
			Task: r.reconcilePrometheusRule,
		},
		{
			Name: "reconcile App NetworkPolicy",
			Task: r.reconcileAppServerNetworkPolicy,
		},
		{
			Name: "reconcile Proxy CA ConfigMap",
			Task: r.reconcileProxyCAConfigMap,
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.logger.Error(err, "reconcileAppServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileAppServer completes")

	return nil
}

func (r *OLSConfigReconciler) reconcileLSCConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {

	return r.reconcileOLSConfigMap(ctx, cr)
	// TODO: implement LSC configmap reconciliation
}

func (r *OLSConfigReconciler) reconcileLSCDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {

	return r.reconcileDeployment(ctx, cr)

	// TODO: implement LSC deployment reconciliation
}
