package controller

import (
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *OLSConfigReconciler) generateRedisDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	cacheReplicas := int32(1)
	DeploymentSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "redis-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-redis",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppRedisDeploymentName,
			Namespace: cr.Namespace,
			Labels:    DeploymentSelectorLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &cacheReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: DeploymentSelectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: DeploymentSelectorLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "lightspeed-redis-server",
							Image:           r.Options.LightspeedServiceRedisImage,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: OLSAppRedisServicePort,
									Name:          "server",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
		},
	}

	return &deployment, nil
}

func (r *OLSConfigReconciler) generateRedisService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	DeploymentSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "redis-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-redis",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppRedisServiceName,
			Namespace: cr.Namespace,
			Labels:    DeploymentSelectorLabels,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       OLSAppRedisServicePort,
					Name:       "server",
					TargetPort: intstr.Parse("server"),
				},
			},
			Selector: DeploymentSelectorLabels,
		},
	}

	return &service, nil
}
