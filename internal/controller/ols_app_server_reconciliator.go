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

func (r *OLSConfigReconciler) reconcileAppServer(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcileAppServer starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile ServiceAccount",
			Task: r.reconcileServiceAccount,
		},
		{
			Name: "reconcile OLSConfigMap",
			Task: r.reconcileOLSConfigMap,
		},
		{
			Name: "reconcile App Deployment",
			Task: r.reconcileDeployment,
		},
		{
			Name: "reconcile App Service",
			Task: r.reconcileService,
		},
		{
			Name: "reconcile Redis Deployment",
			Task: r.reconcileRedisDeployment,
		},
		{
			Name: "reconcile Redis Service",
			Task: r.reconcileRedisService,
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

func (r *OLSConfigReconciler) reconcileOLSConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := r.generateOLSConfigMap(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS configmap: %w", err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSConfigCmName, Namespace: cr.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create OLS configmap: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get OLS configmap: %w", err)
	}

	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("failed to update OLS configmap: %w", err)
	}
	r.stateCache[OLSConfigHashStateCacheKey] = cm.Annotations[OLSConfigHashKey]
	r.logger.Info("OLS configmap reconciled", "configmap", cm.Name, "hash", cm.Annotations[OLSConfigHashKey])
	return nil
}

func (r *OLSConfigReconciler) reconcileServiceAccount(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := r.generateServiceAccount(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS service account: %w", err)
	}

	foundSa := &corev1.ServiceAccount{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceAccountName, Namespace: cr.Namespace}, foundSa)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, sa)
		if err != nil {
			return fmt.Errorf("failed to create OLS service account: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get OLS service account: %w", err)
	}
	r.logger.Info("OLS service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	deployment, err := r.generateOLSDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS deployment: %w", err)
	}

	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: cr.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(deployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
		})
		err = r.Create(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to create OLS deployment: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OLS deployment: %w", err)
	}

	if foundDeployment.Annotations == nil ||
		foundDeployment.Annotations[OLSConfigHashKey] != r.stateCache[OLSConfigHashStateCacheKey] {
		updateDeploymentAnnotations(deployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
		})
		err = r.Update(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to update OLS deployment: %w", err)
		}
		r.logger.Info("OLS deployment reconciled", "deployment", deployment.Name, "olsconfig hash", deployment.Annotations[OLSConfigHashKey])
	} else {

		r.logger.Info("OLS deployment reconciliation skipped", "deployment", foundDeployment.Name, "olsconfig hash", foundDeployment.Annotations[OLSConfigHashKey])
	}

	return nil
}

func (r *OLSConfigReconciler) reconcileService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generateService(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS service: %w", err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerServiceName, Namespace: cr.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create OLS service: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get OLS service: %w", err)
	}
	r.logger.Info("OLS service reconciled", "service", service.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	deployment, err := r.generateRedisDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis deployment: %w", err)
	}

	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppRedisDeploymentName, Namespace: cr.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to create OLS redis deployment: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get OLS redis deployment: %w", err)
	}
	r.logger.Info("OLS redis deployment reconciled", "deployment", deployment.Name)
	return nil
}

func (r *OLSConfigReconciler) reconcileRedisService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generateRedisService(cr)
	if err != nil {
		return fmt.Errorf("failed to generate OLS redis service: %w", err)
	}

	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: OLSAppRedisServiceName, Namespace: cr.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create OLS redis service: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get OLS redis service: %w", err)
	}
	r.logger.Info("OLS redis service reconciled", "service", service.Name)
	return nil
}
