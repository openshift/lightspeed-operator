package controller

import (
	"context"

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

func configMapWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}
	skip := true
	// Check for annotation
	crName, exist := annotations[WatcherAnnotationKey]
	if !exist {
		skip = false
	}
	// Check for name as well. We need a configmap containing a CA bundle that can be used to verify the kube-apiserve
	if obj.GetName() == "kube-root-ca.crt" {
		skip = false
	}

	if skip {
		return nil
	}

	// Update impacted deployment - OLSAppServerDeploymentName
	existingDeployment := &appsv1.Deployment{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: OLSAppServerDeploymentName, Namespace: r.Options.Namespace}, existingDeployment)
	if err != nil{
		
	}

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
