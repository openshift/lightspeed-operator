package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// todo: implement LSC config map generation
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) generateLSCConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) { //lint:ignore U1000 Ignore unused lint error before implementation of reconciliation functions
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerConfigCmName,
			Namespace: r.Options.Namespace,
		},
	}
	return configMap, nil
}

// todo: implement LSC deployment generation
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) generateLSCDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) { //lint:ignore U1000 Ignore unused lint error before implementation of reconciliation functions
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: r.Options.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: cr.Spec.OLSConfig.DeploymentConfig.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: generateAppServerSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: generateAppServerSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "lsc-app-server",
							Image: r.Options.LightspeedServiceImage,
						},
					},
				},
			},
		},
	}
	return deployment, nil
}

// todo: implement LSC deployment update
//
//nolint:unused // Ignore unused lint error before implementation of reconciliation functions
func (r *OLSConfigReconciler) updateLSCDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {

	return nil
}
