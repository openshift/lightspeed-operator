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

func (r *OLSConfigReconciler) reconcileRedisServer(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcileRedisServer starts")
	tasks := []ReconcileTask{
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
			r.logger.Error(err, "reconcileRedisServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileRedisServer completes")

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
