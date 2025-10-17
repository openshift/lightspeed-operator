package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

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
	// TODO: implement LSC configmap reconciliation
	configMap, err := r.generateLSCConfigMap(ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateAPIConfigmap, err)
	}

	foundConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: AppServerConfigCmName, Namespace: r.Options.Namespace}, foundConfigMap)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new LSC configmap", "configmap", configMap.Name)
		err = r.Create(ctx, configMap)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreateAPIConfigmap, err)
		}
		r.logger.Info("LSC configmap created", "configmap", configMap.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetAPIConfigmap, err)
	}
	r.logger.Info("LSC configmap already exists, reconciliation skipped", "configmap", configMap.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileLSCDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	// TODO: implement LSC deployment reconciliation
	r.logger.Info("reconcileLSCDeployment stub called - not yet implemented")
	desiredDeployment, err := r.generateLSCDeployment(ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGenerateAPIDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating a new deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("%s: %w", ErrCreateAPIDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetAPIDeployment, err)
	}

	err = r.updateLSCDeployment(ctx, existingDeployment, desiredDeployment)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateAPIDeployment, err)
	}

	return nil
}
