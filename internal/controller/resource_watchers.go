package controller

import (
	"context"

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
