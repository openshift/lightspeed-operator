package controller

import (
	"context"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func getOLSDataCollectorResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	if cr.Spec.OLSConfig.DeploymentConfig.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.Resources
	}
	// default resources.
	defaultResources := &corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
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
	revisionHistoryLimit := int32(1)
	volumeDefaultMode := int32(420)

	// map from secret name to secret mount path
	secretMounts := map[string]string{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		_, err := getSecretContent(r.Client, provider.CredentialsSecretRef.Name, r.Options.Namespace, []string{LLMApiTokenFileName}, &corev1.Secret{})
		if err != nil {
			return nil, err
		}
		credentialMountPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name)
		secretMounts[provider.CredentialsSecretRef.Name] = credentialMountPath
	}
	// TODO: Update DB
	// Redis volume
	//redisSecretName := cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret
	//redisCredentialsMountPath := path.Join(CredentialsMountRoot, redisSecretName)
	//secretMounts[redisSecretName] = redisCredentialsMountPath

	// TLS volume
	tlsSecretNameMountPath := path.Join(OLSAppCertsMountRoot, OLSCertsSecretName)
	secretMounts[OLSCertsSecretName] = tlsSecretNameMountPath

	// Container ports
	ports := []corev1.ContainerPort{
		{
			ContainerPort: OLSAppServerContainerPort,
			Name:          "https",
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// declare api key secrets and OLS config map as volumes to the pod
	volumes := []corev1.Volume{}
	for secretName := range secretMounts {
		volume := corev1.Volume{
			Name: "secret-" + secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &volumeDefaultMode,
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
				DefaultMode: &volumeDefaultMode,
			},
		},
	}
	olsUserDataVolume := corev1.Volume{
		Name: OLSUserDataVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	// TODO: Update DB
	//volumes = append(volumes, olsConfigVolume, olsUserDataVolume, getRedisCAConfigVolume())
	volumes = append(volumes, olsConfigVolume, olsUserDataVolume)

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
	// TODO: Update DB
	//volumeMounts = append(volumeMounts, olsConfigVolumeMount, olsUserDataVolumeMount, getRedisCAVolumeMount(path.Join(OLSAppCertsMountRoot, RedisCertsSecretName, RedisCAVolume)))
	volumeMounts = append(volumeMounts, olsConfigVolumeMount, olsUserDataVolumeMount)
	replicas := getOLSServerReplicas(cr)
	ols_server_resources := getOLSServerResources(cr)
	data_collector_resources := getOLSDataCollectorResources(cr)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: r.Options.Namespace,
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
							ImagePullPolicy: corev1.PullAlways,
							Ports:           ports,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
							},
							VolumeMounts: volumeMounts,
							Env: append(getProxyEnvVars(), corev1.EnvVar{
								Name:  "OLS_CONFIG_FILE",
								Value: path.Join(OLSConfigMountPath, OLSConfigFilename),
							}),
							Resources: *ols_server_resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/readiness",
										Port:   intstr.FromString("https"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/liveness",
										Port:   intstr.FromString("https"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 120,
								PeriodSeconds:       10,
							},
						},
						{
							Name:            "lightspeed-service-user-data-collector",
							Image:           r.Options.LightspeedServiceImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
							},
							VolumeMounts: []corev1.VolumeMount{olsUserDataVolumeMount},
							Env: []corev1.EnvVar{
								{
									Name:  "OLS_USER_DATA_PATH",
									Value: OLSUserDataMountPath,
								},
								{
									Name:  "INGRESS_ENV",
									Value: "prod",
								},
							},
							Command:   []string{"python3.11", "/app-root/ols/user_data_collection/data_collector.py"},
							Resources: *data_collector_resources,
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
		existingDeployment.Annotations[OLSConfigHashKey] != r.stateCache[OLSConfigHashStateCacheKey] ||
		existingDeployment.Annotations[OLSAppTLSHashKey] != r.stateCache[OLSAppTLSHashStateCacheKey] ||
		existingDeployment.Annotations[LLMProviderHashKey] != r.stateCache[LLMProviderHashStateCacheKey] {
		updateDeploymentAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey:   r.stateCache[OLSConfigHashStateCacheKey],
			OLSAppTLSHashKey:   r.stateCache[OLSAppTLSHashStateCacheKey],
			LLMProviderHashKey: r.stateCache[LLMProviderHashStateCacheKey],
			// TODO: Update DB
			//RedisSecretHashKey: r.stateCache[RedisSecretHashStateCacheKey],
		})
		// update the deployment template annotation triggers the rolling update
		updateDeploymentTemplateAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey:   r.stateCache[OLSConfigHashStateCacheKey],
			OLSAppTLSHashKey:   r.stateCache[OLSAppTLSHashStateCacheKey],
			LLMProviderHashKey: r.stateCache[LLMProviderHashStateCacheKey],
			// TODO: Update DB
			//RedisSecretHashKey: r.stateCache[RedisSecretHashStateCacheKey],
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
