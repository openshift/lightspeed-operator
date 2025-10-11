package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func secretWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}
	crName, exist := annotations[WatcherAnnotationKey]
	if !exist {
		return nil
	}
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: crName,
		}},
	}
}

func annotateSecretWatcher(secret *corev1.Secret) {
	annotations := secret.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[WatcherAnnotationKey] = OLSConfigName
	secret.SetAnnotations(annotations)
}

func telemetryPullSecretWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() != TelemetryPullSecretNamespace || obj.GetName() != TelemetryPullSecretName {
		return nil
	}
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: OLSConfigName,
		}},
	}
}

func (r *OLSConfigReconciler) configMapWatcherFilter(ctx context.Context, obj client.Object, inCluster ...bool) []reconcile.Request {

	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	skip := true
	// Check for annotation
	crName, exist := annotations[WatcherAnnotationKey]
	if exist {
		skip = false
	}
	// Check for name as well. We need a configmap containing a CA bundle that can be used to verify the kube-apiserve
	if obj.GetName() == DefaultOpenShiftCerts {
		crName = OLSConfigName
		skip = false
	}

	if skip {
		return nil
	}

	// Restart server
	err := r.restartAppServer(ctx, inClusterValue)
	if err != nil {
		return nil
	}

	// Reconsile request
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: crName,
		}},
	}
}

func annotateConfigMapWatcher(cm *corev1.ConfigMap) {
	annotations := cm.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[WatcherAnnotationKey] = OLSConfigName
	cm.SetAnnotations(annotations)
}

func (r *OLSConfigReconciler) restartAppServer(ctx context.Context, inCluster bool) error {

	if inCluster {
		// Update impacted deployment - OLSAppServerDeploymentName
		dep := &appsv1.Deployment{}
		err := r.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: r.Options.Namespace}, dep)
		if err != nil {
			r.logger.Info("failed to get deployment", "deploymentName", OLSAppServerDeploymentName, "error", err)
			return err
		}
		// init map if empty
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = make(map[string]string)
		}
		// bump the annotation → new template hash → rolling update
		dep.Spec.Template.Annotations[ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)
		// Update
		r.logger.Info("updating OLS deployment", "name", dep.Name)
		err = r.Update(ctx, dep)
		if err != nil {
			r.logger.Info("failed to update deployment", "deploymentName", dep.Name, "error", err)
			return err
		}
	}
	return nil
}
