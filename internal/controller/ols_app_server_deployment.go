package controller

import (
	"context"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func getOLSServerReplicas(cr *olsv1alpha1.OLSConfig) *int32 {
	if cr.Spec.OLSConfig.DeploymentConfig.Replicas != nil && *cr.Spec.OLSConfig.DeploymentConfig.Replicas >= 0 {
		return cr.Spec.OLSConfig.DeploymentConfig.Replicas
	}
	// default number of replicas.
	defaultReplicas := int32(1)
	return &defaultReplicas
}

func getOLSServerResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	if cr.Spec.OLSConfig.DeploymentConfig.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.Resources
	}
	// default resources.
	defaultResources := &corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("2Gi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("1Gi")},
		Claims:   []corev1.ResourceClaim{},
	}

	return defaultResources
}

func (r *OLSConfigReconciler) generateOLSDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	// mount points of API key secret
	const OLSConfigMountPath = "/etc/ols"
	const OLSConfigVolumeName = "cm-olsconfig"
	const OLSUserDataVolumeName = "ols-user-data"
	const OLSUserDataMountPath = "/app-root/ols-user-data"
	revisionHistoryLimit := int32(0)

	// map from secret name to secret mount path
	secretMounts := map[string]string{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialMountPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name)
		secretMounts[provider.CredentialsSecretRef.Name] = credentialMountPath
	}

	redisSecretName := cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecretRef.Name
	redisCredentialsMountPath := path.Join(CredentialsMountRoot, redisSecretName)
	secretMounts[redisSecretName] = redisCredentialsMountPath
	// declare api key secrets and OLS config map as volumes to the pod
	volumes := []corev1.Volume{}
	for secretName := range secretMounts {
		volume := corev1.Volume{
			Name: "secret-" + secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: secretName,
				},
			},
		}
		volumes = append(volumes, volume)
	}
	olsConfigVolume := corev1.Volume{
		Name: OLSConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OLSConfigCmName,
				},
			},
		},
	}
	olsUserDataVolume := corev1.Volume{
		Name: OLSUserDataVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	redisCAConfigVolume := corev1.Volume{
		Name: RedisCAVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: RedisCAConfigMap,
				},
			},
		},
	}
	volumes = append(volumes, olsConfigVolume, olsUserDataVolume, redisCAConfigVolume)

	// mount the volumes of api keys secrets and OLS config map to the container
	volumeMounts := []corev1.VolumeMount{}
	for secretName, mountPath := range secretMounts {
		volumeMount := corev1.VolumeMount{
			Name:      "secret-" + secretName,
			MountPath: mountPath,
			ReadOnly:  true,
		}
		volumeMounts = append(volumeMounts, volumeMount)
	}
	olsConfigVolumeMount := corev1.VolumeMount{
		Name:      OLSConfigVolumeName,
		MountPath: OLSConfigMountPath,
		ReadOnly:  true,
	}
	olsUserDataVolumeMount := corev1.VolumeMount{
		Name:      OLSUserDataVolumeName,
		MountPath: OLSUserDataMountPath,
	}
	olsRedisCAVolumeMount := corev1.VolumeMount{
		Name:      RedisCAVolume,
		MountPath: path.Join(OLSAppCertsMountRoot, RedisCertsSecretName, RedisCAVolume),
		ReadOnly:  true,
	}
	volumeMounts = append(volumeMounts, olsConfigVolumeMount, olsUserDataVolumeMount, olsRedisCAVolumeMount)

	replicas := getOLSServerReplicas(cr)
	resources := getOLSServerResources(cr)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: cr.Namespace,
			Labels:    generateAppServerSelectorLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
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
							Name:            "lightspeed-service-api",
							Image:           r.Options.LightspeedServiceImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: OLSAppServerContainerPort,
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
							},
							VolumeMounts: volumeMounts,
							Env: []corev1.EnvVar{
								{
									Name:  "OLS_CONFIG_FILE",
									Value: path.Join(OLSConfigMountPath, OLSConfigFilename),
								},
							},
							Resources: *resources,
						},
					},
					Volumes:            volumes,
					ServiceAccountName: OLSAppServerServiceAccountName,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &deployment, r.Scheme); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// updateOLSDeployment updates the deployment based on CustomResource configuration.
func (r *OLSConfigReconciler) updateOLSDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	changed := false

	// Validate deployment annotations.
	if existingDeployment.Annotations == nil ||
		existingDeployment.Annotations[OLSConfigHashKey] != r.stateCache[OLSConfigHashStateCacheKey] || existingDeployment.Annotations[RedisSecretHashKey] != r.stateCache[RedisSecretHashStateCacheKey] {
		updateDeploymentAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey:   r.stateCache[OLSConfigHashStateCacheKey],
			RedisSecretHashKey: r.stateCache[RedisSecretHashStateCacheKey],
		})
		// update the deployment template annotation triggers the rolling update
		updateDeploymentTemplateAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey:   r.stateCache[OLSConfigHashStateCacheKey],
			RedisSecretHashKey: r.stateCache[RedisSecretHashStateCacheKey],
		})

		changed = true
	}

	// Validate deployment replicas.
	if setDeploymentReplicas(existingDeployment, *desiredDeployment.Spec.Replicas) {
		changed = true
	}

	// Validate deployment volumes.
	if setVolumes(existingDeployment, desiredDeployment.Spec.Template.Spec.Volumes) {
		changed = true
	}

	// Validate volume mounts for a specific container in deployment.
	if volumeMountsChanged, err := setVolumeMounts(existingDeployment, desiredDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, "lightspeed-service-api"); err != nil {
		return err
	} else if volumeMountsChanged {
		changed = true
	}

	// Validate deployment resources.
	if resourcesChanged, err := setDeploymentContainerResources(existingDeployment, &desiredDeployment.Spec.Template.Spec.Containers[0].Resources, "lightspeed-service-api"); err != nil {
		return err
	} else if resourcesChanged {
		changed = true
	}

	// validate volumes including token secrets and application config map
	if !podVolumeEqual(existingDeployment.Spec.Template.Spec.Volumes, desiredDeployment.Spec.Template.Spec.Volumes) {
		changed = true
		existingDeployment.Spec.Template.Spec.Volumes = desiredDeployment.Spec.Template.Spec.Volumes
		_, err := setDeploymentContainerVolumeMounts(existingDeployment, "lightspeed-service-api", desiredDeployment.Spec.Template.Spec.Containers[0].VolumeMounts)
		if err != nil {
			return err
		}
	}

	if changed {
		r.logger.Info("updating OLS deployment", "name", existingDeployment.Name)
		if err := r.Update(ctx, existingDeployment); err != nil {
			return err
		}
	} else {
		r.logger.Info("OLS deployment reconciliation skipped", "deployment", existingDeployment.Name, "olsconfig hash", existingDeployment.Annotations[OLSConfigHashKey])
	}

	return nil
}
