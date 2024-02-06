package controller

import (
	"fmt"
	"path"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

func (r *OLSConfigReconciler) generateServiceAccount(cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerServiceAccountName,
			Namespace: cr.Namespace,
		},
	}
	return &sa, nil
}

func (r *OLSConfigReconciler) generateOLSConfigMap(cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {

	providerConfigs := []ProviderConfig{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name, LLMApiTokenFileName)
		modelConfigs := []ModelConfig{}
		for _, model := range provider.Models {
			modelConfig := ModelConfig{
				Name: model.Name,
				URL:  model.URL,
			}
			modelConfigs = append(modelConfigs, modelConfig)
		}

		providerConfig := ProviderConfig{
			Name:            provider.Name,
			URL:             provider.URL,
			CredentialsPath: credentialPath,
			Models:          modelConfigs,
		}
		providerConfigs = append(providerConfigs, providerConfig)
	}

	conversationCache := ConversationCacheConfig{
		Type:   string(cr.Spec.OLSConfig.ConversationCache.Type),
		Memory: MemoryCacheConfig{MaxEntries: cr.Spec.OLSConfig.ConversationCache.Memory.MaxEntries},
	}

	olsConfig := OLSConfig{
		DefaultModel:    cr.Spec.OLSConfig.DefaultModel,
		DefaultProvider: cr.Spec.OLSConfig.DefaultProvider,
		Logging: LoggingConfig{
			AppLogLevel: cr.Spec.OLSConfig.LogLevel,
			LibLogLevel: cr.Spec.OLSConfig.LogLevel,
		},
		ConversationCache: conversationCache,
	}

	appSrvConfigFile := AppSrvConfigFile{
		LLMProviders: providerConfigs,
		OLSConfig:    olsConfig,
	}
	configFileBytes, err := yaml.Marshal(appSrvConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file %w", err)
	}

	configFileHash, err := hashBytes(configFileBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS config file hash %w", err)
	}

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSConfigCmName,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":  "application-server",
				"app.kubernetes.io/managed-by": "lightspeed-operator",
				"app.kubernetes.io/name":       "lightspeed-service-api",
				"app.kubernetes.io/part-of":    "openshift-lightspeed",
			},
			Annotations: map[string]string{
				OLSConfigHashKey: configFileHash,
			},
		},
		Data: map[string]string{
			OLSConfigFilename: string(configFileBytes),
		},
	}

	return &cm, nil
}

func (r *OLSConfigReconciler) generateOLSDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {

	// mount points of API key secret
	const OLSConfigMountPath = "/etc/ols"
	const OLSConfigVolumeName = "cm-olsconfig"
	DeploymentSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}

	// map from secret name to secret mount path
	secretMounts := map[string]string{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialMountPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name)
		secretMounts[provider.CredentialsSecretRef.Name] = credentialMountPath
	}

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
	volume := corev1.Volume{
		Name: OLSConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OLSConfigCmName,
				},
			},
		},
	}
	volumes = append(volumes, volume)

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
	volumeMount := corev1.VolumeMount{
		Name:      OLSConfigVolumeName,
		MountPath: OLSConfigMountPath,
		ReadOnly:  true,
	}
	volumeMounts = append(volumeMounts, volumeMount)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: cr.Namespace,
			Labels:    DeploymentSelectorLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &cr.Spec.OLSConfig.DeploymentConfig.Replicas,
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
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
					Volumes:            volumes,
					ServiceAccountName: OLSAppServerServiceAccountName,
				},
			},
		},
	}

	return &deployment, nil
}

func (r *OLSConfigReconciler) generateService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	DeploymentSelectorLabels := map[string]string{
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: cr.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":  "application-server",
				"app.kubernetes.io/managed-by": "lightspeed-operator",
				"app.kubernetes.io/name":       "lightspeed-service-api",
				"app.kubernetes.io/part-of":    "openshift-lightspeed",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       OLSAppServerServicePort,
					Name:       "http",
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("http"),
				},
			},
			Selector: DeploymentSelectorLabels,
		},
	}

	return &service, nil
}
