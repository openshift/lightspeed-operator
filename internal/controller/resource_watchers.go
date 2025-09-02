package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func secretWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	// Try each specific watcher in order
	if requests := postgresCASecretWatcher(ctx, obj); len(requests) > 0 {
		return requests
	}

	if requests := telemetryPullSecretWatcher(ctx, obj); len(requests) > 0 {
		return requests
	}

	if requests := annotatedSecretWatcher(ctx, obj); len(requests) > 0 {
		return requests
	}

	return nil
}

func postgresCASecretWatcher(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetName() == PostgresCertsSecretName && obj.GetNamespace() == "openshift-lightspeed" {
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Name: OLSConfigName,
			}},
		}
	}
	return nil
}

func telemetryPullSecretWatcher(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetNamespace() == TelemetryPullSecretNamespace && obj.GetName() == TelemetryPullSecretName {
		return []reconcile.Request{
			{NamespacedName: types.NamespacedName{
				Name: OLSConfigName,
			}},
		}
	}
	return nil
}

func annotatedSecretWatcher(ctx context.Context, obj client.Object) []reconcile.Request {
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if crName, exist := annotations[WatcherAnnotationKey]; exist {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name: crName,
				}},
			}
		}
	}
	return nil
}

func annotateSecretWatcher(secret *corev1.Secret) {
	annotations := secret.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[WatcherAnnotationKey] = OLSConfigName
	secret.SetAnnotations(annotations)
}

func configMapWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	// Check for OpenShift service CA configmap first
	if obj.GetName() == "openshift-service-ca.crt" && obj.GetNamespace() == "openshift-config" {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: OLSConfigName}}}
	}

	// Check for annotated configmaps
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if crName, exist := annotations[WatcherAnnotationKey]; exist {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name: crName,
				}},
			}
		}
	}
	return nil
}

func annotateConfigMapWatcher(cm *corev1.ConfigMap) {
	annotations := cm.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[WatcherAnnotationKey] = OLSConfigName
	cm.SetAnnotations(annotations)
}

func postgresCAConfigMapWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	if obj.GetName() != OLSCAConfigMap {
		return nil
	}
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: OLSConfigName,
		}},
	}
}
