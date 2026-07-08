package alertsadapter

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func getAlertsAdapterResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	var userResources *corev1.ResourceRequirements
	if cr.Spec.OLSConfig.DeploymentConfig.AlertsAdapter != nil {
		userResources = cr.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.Resources
	}
	return utils.GetResourcesOrDefault(
		userResources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("50Mi"),
			},
		},
	)
}

// GenerateDeployment generates the alerts adapter Deployment.
// When the referenced user ConfigMap exists, it is mounted read-only at /etc/alerts-adapter.
// If configMapRef is set but the ConfigMap is absent, no config volume is mounted and the adapter uses defaults.
func GenerateDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	runAsNonRoot := true
	resources := getAlertsAdapterResources(cr)

	volumes := []corev1.Volume{
		{
			Name: utils.TmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      utils.TmpVolumeName,
			MountPath: utils.TmpVolumeMountPath,
		},
	}

	if _, ok := utils.AlertsAdapterConfigMapRef(cr); ok {
		cm, err := getUserConfigMap(r, ctx, cr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterDeployment, err)
		}
		if cm != nil {
			volumes = append(volumes, corev1.Volume{
				Name: utils.AlertsAdapterConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: cm.Name},
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      utils.AlertsAdapterConfigVolumeName,
				MountPath: utils.AlertsAdapterConfigVolumeMountPath,
				ReadOnly:  true,
			})
		}
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AlertsAdapterDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAlertsAdapterSelectorLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: utils.GenerateAlertsAdapterSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: utils.GenerateAlertsAdapterSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: utils.AlertsAdapterServiceAccountName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            utils.AlertsAdapterContainerName,
							Image:           r.GetAlertsAdapterImage(),
							ImagePullPolicy: corev1.PullAlways,
							SecurityContext: utils.RestrictedContainerSecurityContext(),
							Resources:       *resources,
							Env: append(utils.GetProxyEnvVars(),
								corev1.EnvVar{
									Name:  utils.AlertsAdapterAlertmanagerURLEnvVar,
									Value: utils.AlertsAdapterAlertmanagerURL,
								},
								corev1.EnvVar{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											APIVersion: "v1",
											FieldPath:  "metadata.namespace",
										},
									},
								},
							),
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	var config olsv1alpha1.Config
	if cr.Spec.OLSConfig.DeploymentConfig.AlertsAdapter != nil {
		config = cr.Spec.OLSConfig.DeploymentConfig.AlertsAdapter.Config
	}
	utils.ApplyPodDeploymentConfig(deployment, config, false)

	if err := controllerutil.SetControllerReference(cr, deployment, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetAlertsAdapterDeploymentOwnerReference, err)
	}

	return deployment, nil
}
