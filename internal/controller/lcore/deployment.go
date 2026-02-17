package lcore

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// getLlamaStackResources returns resource requirements for the llama-stack container
// This container runs the Llama Stack inference service (sidecar to lightspeed-stack)
func getLlamaStackResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	// llama-stack is a sidecar inference backend with fixed resource requirements
	// It always gets default resources to ensure stable inference performance
	// Users can configure the main API container (lightspeed-stack) via APIContainer.Resources
	// Note: The pod must have enough resources to accommodate both containers
	//
	// TODO: Consider adding LlamaStackContainerConfig to the API in a future PR to allow
	// users to configure llama-stack resources independently of the main API container.
	// This would follow the same pattern as APIContainer, DataCollectorContainer, etc.
	defaultResources := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			corev1.ResourceCPU:    resource.MustParse("1000m"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Claims: []corev1.ResourceClaim{},
	}

	return utils.GetResourcesOrDefault(cr.Spec.OLSConfig.DeploymentConfig.LlamaStackContainer.Resources, defaultResources)
}

// getLightspeedStackResources returns resource requirements for the lightspeed-stack container
// This is the main API container serving user requests
func getLightspeedStackResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Resources,
		&corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				corev1.ResourceCPU:    resource.MustParse("1000m"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
			Claims: []corev1.ResourceClaim{},
		},
	)
}

// getOLSMCPServerResources returns resource requirements for the OpenShift MCP server sidecar
func getOLSMCPServerResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.MCPServerContainer.Resources,
		&corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
			Claims:   []corev1.ResourceClaim{},
		},
	)
}

// getOLSDataCollectorResources returns resource requirements for the data collector sidecar
func getOLSDataCollectorResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.DataCollectorContainer.Resources,
		&corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
			Claims:   []corev1.ResourceClaim{},
		},
	)
}

// addOpenShiftMCPServerSidecar adds the OpenShift MCP server sidecar container to the deployment
// if introspection is enabled in the CR. This modifies the deployment in place.
func addOpenShiftMCPServerSidecar(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig, deployment *appsv1.Deployment) {
	if !cr.Spec.OLSConfig.IntrospectionEnabled {
		return
	}

	openshiftMCPServerContainer := corev1.Container{
		Name:            utils.OpenShiftMCPServerContainerName,
		Image:           r.GetOpenShiftMCPServerImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &[]bool{false}[0],
			ReadOnlyRootFilesystem:   &[]bool{true}[0],
		},
		Command: []string{
			"/openshift-mcp-server",
			"--read-only",
			"--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerPort),
		},
		Resources: *getOLSMCPServerResources(cr),
	}

	deployment.Spec.Template.Spec.Containers = append(
		deployment.Spec.Template.Spec.Containers,
		openshiftMCPServerContainer,
	)
}

// addDataCollectorSidecar adds the data collector container to the deployment if data collection is enabled
func addDataCollectorSidecar(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig, deployment *appsv1.Deployment, volumeMounts []corev1.VolumeMount, dataCollectorEnabled bool) {
	if !dataCollectorEnabled {
		return
	}

	// Get log level from CR
	logLevel := cr.Spec.OLSDataCollectorConfig.LogLevel
	if logLevel == "" {
		logLevel = olsv1alpha1.LogLevelInfo
	}

	exporterContainer := corev1.Container{
		Name:            "lightspeed-to-dataverse-exporter",
		Image:           r.GetDataverseExporterImage(),
		ImagePullPolicy: corev1.PullAlways,
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &[]bool{false}[0],
			ReadOnlyRootFilesystem:   &[]bool{true}[0],
		},
		VolumeMounts: volumeMounts,
		// running in openshift mode ensures that cluster_id is set
		// as identity_id
		Args: []string{
			"--mode",
			"openshift",
			"--config",
			path.Join(utils.ExporterConfigMountPath, utils.ExporterConfigFilename),
			"--log-level",
			string(logLevel),
			"--data-dir",
			utils.LCoreUserDataMountPath,
		},
		Resources: *getOLSDataCollectorResources(cr),
	}

	deployment.Spec.Template.Spec.Containers = append(
		deployment.Spec.Template.Spec.Containers,
		exporterContainer,
	)
}

// buildLlamaStackEnvVars builds environment variables for all LLM providers
// For Azure providers, it reads the secret to support both API key and client credentials
func buildLlamaStackEnvVars(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{}

	// Add environment variables for each LLM provider secret using iterator
	err := utils.ForEachExternalSecret(cr, func(name, source string) error {
		if !strings.HasPrefix(source, "llm-provider-") {
			return nil
		}

		// Extract provider name from source (format: "llm-provider-<name>")
		providerName := strings.TrimPrefix(source, "llm-provider-")
		envVarBase := utils.ProviderNameToEnvVarName(providerName)

		// Find the provider in the CR to check its type
		var provider *olsv1alpha1.ProviderSpec
		for i := range cr.Spec.LLMConfig.Providers {
			if cr.Spec.LLMConfig.Providers[i].Name == providerName {
				provider = &cr.Spec.LLMConfig.Providers[i]
				break
			}
		}

		// For Azure providers, read the secret to support both authentication methods
		if provider != nil && provider.Type == "azure_openai" {
			secret := &corev1.Secret{}
			err := r.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: r.GetNamespace(),
			}, secret)
			if err != nil {
				return fmt.Errorf("failed to get secret %s: %w", name, err)
			}

			// Create environment variables for each key in the secret
			// Azure supports both API key (apitoken) and client credentials (client_id, tenant_id, client_secret)
			keyToEnvSuffix := map[string]string{
				"apitoken":      "_API_KEY",
				"client_id":     "_CLIENT_ID",
				"tenant_id":     "_TENANT_ID",
				"client_secret": "_CLIENT_SECRET",
			}

			for key := range secret.Data {
				if suffix, ok := keyToEnvSuffix[key]; ok {
					envVars = append(envVars, corev1.EnvVar{
						Name: envVarBase + suffix,
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: name},
								Key:                  key,
							},
						},
					})
				}
			}

			// LiteLLM requires api_key field to be present in the config for Azure
			// even when using client credentials authentication. Check if we have an API_KEY env var,
			// and if not, add one with a placeholder value to satisfy LiteLLM's Pydantic validation.
			hasAPIKey := false
			apiKeyEnvName := envVarBase + "_API_KEY"
			for _, env := range envVars {
				if env.Name == apiKeyEnvName {
					hasAPIKey = true
					break
				}
			}
			if !hasAPIKey {
				envVars = append(envVars, corev1.EnvVar{
					Name:  apiKeyEnvName,
					Value: "placeholder",
				})
			}
		} else {
			// For non-Azure providers, always use API key
			envVars = append(envVars, corev1.EnvVar{
				Name: envVarBase + "_API_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: name},
						Key:                  "apitoken",
					},
				},
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Add PostgreSQL password environment variable
	envVars = append(envVars, corev1.EnvVar{
		Name: "POSTGRES_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: utils.PostgresSecretName},
				Key:                  utils.OLSComponentPasswordFileName,
			},
		},
	})

	return envVars, nil
}

// buildLightspeedStackEnvVars builds environment variables for the lightspeed-stack container
func buildLightspeedStackEnvVars(_ reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name: "LOG_LEVEL",
			Value: func() string {
				if cr.Spec.OLSConfig.LogLevel != "" {
					return string(cr.Spec.OLSConfig.LogLevel)
				}
				return string(olsv1alpha1.LogLevelInfo)
			}(),
		},
		// PostgreSQL password for database configuration
		{
			Name: "POSTGRES_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: utils.PostgresSecretName,
					},
					Key: utils.OLSComponentPasswordFileName,
				},
			},
		},
	}

	return envVars
}

// validateMCPHeaderSecret validates that a secret exists and has the required structure for MCP headers.
// This provides fail-fast validation consistent with AppServer's approach.
//
// Parameters:
//   - r: Reconciler for K8s API access and logging
//   - ctx: Context for the K8s API call
//   - secretRef: Name of the secret to validate
//   - serverName: Name of the MCP server (for error messages)
//   - headerName: Name of the header (for error messages)
//
// Returns:
//   - error if secret doesn't exist or has incorrect structure
func validateMCPHeaderSecret(r reconciler.Reconciler, ctx context.Context, secretRef, serverName, headerName string) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretRef, Namespace: r.GetNamespace()}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Error(err, "MCP header secret not found",
				"server", serverName,
				"secret", secretRef,
				"header", headerName)
			return fmt.Errorf("MCP server %s header secret %s is not found", serverName, secretRef)
		}
		r.GetLogger().Error(err, "Failed to get MCP header secret",
			"server", serverName,
			"secret", secretRef,
			"header", headerName)
		return fmt.Errorf("failed to get secret %s for MCP server %s: %w", secretRef, serverName, err)
	}

	// Validate secret has the required "header" key (consistent with AppServer)
	if _, ok := secret.Data[utils.MCPSECRETDATAPATH]; !ok {
		err := fmt.Errorf("secret missing required key '%s'", utils.MCPSECRETDATAPATH)
		r.GetLogger().Error(err, "MCP header secret has incorrect structure",
			"server", serverName,
			"secret", secretRef,
			"header", headerName,
			"requiredKey", utils.MCPSECRETDATAPATH)
		return fmt.Errorf("header secret %s for MCP server %s is missing key '%s'", secretRef, serverName, utils.MCPSECRETDATAPATH)
	}

	return nil
}

// ============================================================================
// Helper functions for building common deployment components
// ============================================================================

// buildCommonLabels returns the standard labels for LCore deployments
func buildCommonLabels() map[string]string {
	return map[string]string{
		"app":                          "lightspeed-stack",
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

// buildConfigVolumes creates the base config volumes for LCore (both server and library modes need both configs)
// buildLCoreConfigVolumeAndMount creates both the volume and volume mount for lightspeed-stack config
func buildLCoreConfigVolumeAndMount(volumeDefaultMode *int32) (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: "lightspeed-stack-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: utils.LCoreConfigCmName,
				},
				DefaultMode: volumeDefaultMode,
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      "lightspeed-stack-config",
		MountPath: utils.LCoreConfigMountPath,
		SubPath:   utils.LCoreConfigFilename,
		ReadOnly:  true,
	}

	return volume, volumeMount
}

// buildLlamaStackConfigVolumeAndMount creates both the volume and volume mount for llama-stack config
func buildLlamaStackConfigVolumeAndMount(volumeDefaultMode *int32) (corev1.Volume, corev1.VolumeMount) {
	volume := corev1.Volume{
		Name: "llama-stack-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: utils.LlamaStackConfigCmName,
				},
				DefaultMode: volumeDefaultMode,
			},
		},
	}

	volumeMount := corev1.VolumeMount{
		Name:      "llama-stack-config",
		MountPath: utils.LlamaStackConfigMountPath,
		SubPath:   utils.LlamaStackConfigFilename,
		ReadOnly:  true,
	}

	return volume, volumeMount
}

// addTLSVolumesAndMounts adds TLS certificate volumes and mounts if not using custom TLS
func addTLSVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, cr *olsv1alpha1.OLSConfig, volumeDefaultMode *int32) {
	usesCustomTLS := cr.Spec.OLSConfig.TLSSecurityProfile != nil && string(cr.Spec.OLSConfig.TLSSecurityProfile.Type) == "Custom"
	if !usesCustomTLS {
		*volumes = append(*volumes, corev1.Volume{
			Name: "secret-lightspeed-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.OLSCertsSecretName,
					DefaultMode: volumeDefaultMode,
				},
			},
		})
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      "secret-lightspeed-tls",
			MountPath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls"),
			ReadOnly:  true,
		})
	}
}

// addOpenShiftCAVolumesAndMounts adds OpenShift service CA bundle volumes and mounts
func addOpenShiftCAVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, volumeDefaultMode *int32) {
	*volumes = append(*volumes, corev1.Volume{
		Name: "openshift-service-ca",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: utils.OLSCAConfigMap,
				},
				DefaultMode: volumeDefaultMode,
			},
		},
	})
	*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
		Name:      "openshift-service-ca",
		MountPath: "/etc/certs/service-ca",
		ReadOnly:  true,
	})
}

// addOpenShiftRootCAVolumesAndMounts adds OpenShift root CA (kube-root-ca.crt) volumes and mounts
func addOpenShiftRootCAVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, volumeDefaultMode *int32) {
	*volumes = append(*volumes, corev1.Volume{
		Name: utils.OpenShiftCAVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "kube-root-ca.crt",
				},
				DefaultMode: volumeDefaultMode,
			},
		},
	})
	*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
		Name:      utils.OpenShiftCAVolumeName,
		MountPath: "/etc/pki/ca-trust/extracted/pem",
		ReadOnly:  true,
	})
}

// addLlamaCacheVolumesAndMounts adds llama-cache EmptyDir volume and mount for Llama Stack workspace
func addLlamaCacheVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount) {
	*volumes = append(*volumes, corev1.Volume{
		Name: "llama-cache",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
	*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
		Name:      "llama-cache",
		MountPath: "/app-root/.llama",
		ReadOnly:  false,
	})
}

// addPostgresCAVolumesAndMounts adds PostgreSQL CA ConfigMap volume and mount for TLS verification
func addPostgresCAVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, mountPath string) {
	*volumes = append(*volumes, utils.GetPostgresCAConfigVolume())
	*volumeMounts = append(*volumeMounts, utils.GetPostgresCAVolumeMount(mountPath))
}

// addUserCAVolumesAndMounts adds user-provided CA certificate volumes and mounts
// Mounts at /etc/pki/ca-trust/source/anchors (system trust store path)
func addUserCAVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, cr *olsv1alpha1.OLSConfig, volumeDefaultMode *int32) {
	_ = utils.ForEachExternalConfigMap(cr, func(name, source string) error {
		if source != "additional-ca" {
			return nil
		}

		*volumes = append(*volumes, corev1.Volume{
			Name: utils.AdditionalCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					DefaultMode: volumeDefaultMode,
				},
			},
		})
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      utils.AdditionalCAVolumeName,
			MountPath: "/etc/pki/ca-trust/source/anchors",
			ReadOnly:  true,
		})
		return nil
	})
}

// addProxyCACertVolumeAndMount adds the proxy CA ConfigMap volume and mount if a proxy CA is configured.
func addProxyCACertVolumeAndMount(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, cr *olsv1alpha1.OLSConfig, volumeDefaultMode *int32) {
	if cr.Spec.OLSConfig.ProxyConfig == nil {
		return
	}
	proxyCACertRef := cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef
	cmName := utils.GetProxyCACertConfigMapName(proxyCACertRef)
	if cmName == "" {
		return
	}
	certKey := utils.GetProxyCACertKey(proxyCACertRef)
	*volumes = append(*volumes, corev1.Volume{
		Name: utils.ProxyCACertVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
				DefaultMode:          volumeDefaultMode,
				Items: []corev1.KeyToPath{
					{Key: certKey, Path: certKey},
				},
			},
		},
	})
	*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
		Name:      utils.ProxyCACertVolumeName,
		MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName),
		ReadOnly:  true,
	})
}

// addCustomTLSVolumesAndMounts adds user-provided custom TLS certificate volumes and mounts if specified
func addCustomTLSVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, cr *olsv1alpha1.OLSConfig, volumeDefaultMode *int32) {
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		*volumes = append(*volumes, corev1.Volume{
			Name: "secret-lightspeed-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name,
					DefaultMode: volumeDefaultMode,
				},
			},
		})
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      "secret-lightspeed-tls",
			MountPath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls"),
			ReadOnly:  true,
		})
	}
}

// addMCPHeaderSecretVolumesAndMounts adds MCP header secret volumes and mounts for MCP servers
// This validates and mounts header secrets at /etc/mcp/headers/<secretName>
func addMCPHeaderSecretVolumesAndMounts(r reconciler.Reconciler, ctx context.Context, volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, cr *olsv1alpha1.OLSConfig, volumeDefaultMode *int32) error {
	// Only add MCP header secrets if feature gate is enabled
	if cr.Spec.FeatureGates == nil || !slices.Contains(cr.Spec.FeatureGates, utils.FeatureGateMCPServer) {
		return nil
	}

	// Mount MCP header secrets using the same pattern as appserver
	_ = utils.ForEachExternalSecret(cr, func(name, source string) error {
		if strings.HasPrefix(source, "mcp-") {
			// Validate secret exists and has correct structure
			serverName := strings.TrimPrefix(source, "mcp-")
			if err := validateMCPHeaderSecret(r, ctx, name, serverName, ""); err != nil {
				return err
			}

			*volumes = append(*volumes, corev1.Volume{
				Name: "header-" + name,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  name,
						DefaultMode: volumeDefaultMode,
					},
				},
			})

			*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
				Name:      "header-" + name,
				MountPath: path.Join(utils.MCPHeadersMountRoot, name),
				ReadOnly:  true,
			})
		}
		return nil
	})

	return nil
}

// addDataCollectorVolumesAndMounts adds volumes and mounts needed for data collection (feedback/transcripts)
// This creates a shared EmptyDir volume for OLS to write user data and the exporter to read it
func addDataCollectorVolumesAndMounts(volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount, volumeDefaultMode *int32, dataCollectorEnabled bool) {
	if !dataCollectorEnabled {
		return
	}

	// Shared EmptyDir volume for user data (feedback and transcripts)
	*volumes = append(*volumes, corev1.Volume{
		Name: "ols-user-data",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})

	*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
		Name:      "ols-user-data",
		MountPath: utils.LCoreUserDataMountPath,
	})

	// Exporter config volume (ConfigMap)
	*volumes = append(*volumes, corev1.Volume{
		Name: utils.ExporterConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: utils.ExporterConfigCmName,
				},
				DefaultMode: volumeDefaultMode,
			},
		},
	})

	*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
		Name:      utils.ExporterConfigVolumeName,
		MountPath: utils.ExporterConfigMountPath,
		ReadOnly:  true,
	})
}

// buildLightspeedStackLivenessProbe creates the liveness probe for lightspeed-stack container
func buildLightspeedStackLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"sh",
					"-c",
					"curl -k --fail -H \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" https://localhost:8443/liveness",
				},
			},
		},
		InitialDelaySeconds: 20,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
	}
}

// buildLightspeedStackReadinessProbe creates the readiness probe for lightspeed-stack container
func buildLightspeedStackReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"sh",
					"-c",
					"curl -k --fail -H \"Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)\" https://localhost:8443/readiness",
				},
			},
		},
		InitialDelaySeconds: 20,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
	}
}

// buildLightspeedStackContainer creates the base lightspeed-stack container
func buildLightspeedStackContainer(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar) corev1.Container {
	lightspeedStackResources := getLightspeedStackResources(cr)

	return corev1.Container{
		Name:            "lightspeed-service-api",
		Image:           r.GetLCoreImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: utils.OLSAppServerContainerPort,
				Name:          "https",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:            envVars,
		VolumeMounts:   volumeMounts,
		Resources:      *lightspeedStackResources,
		LivenessProbe:  buildLightspeedStackLivenessProbe(),
		ReadinessProbe: buildLightspeedStackReadinessProbe(),
	}
}

// ============================================================================
// Deployment generation functions
// ============================================================================

// GenerateLCoreDeployment generates the Deployment for LCore based on the server mode
func GenerateLCoreDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	if r.GetLCoreServerMode() {
		return generateLCoreServerDeployment(r, cr)
	}
	return generateLCoreLibraryDeployment(r, cr)
}

// generateLCoreServerDeployment generates the Deployment for LCore in server mode (llama-stack + lightspeed-stack)
func generateLCoreServerDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	ctx := context.Background()
	revisionHistoryLimit := int32(1)
	volumeDefaultMode := utils.VolumeDefaultMode

	// Check if data collector is enabled
	dataCollectorEnabled, err := dataCollectorEnabled(r, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to check data collector status: %w", err)
	}

	llamaStackResources := getLlamaStackResources(cr)
	lightspeedStackResources := getLightspeedStackResources(cr)

	// Get ResourceVersions for tracking - these resources should already exist
	// If they don't exist, we'll get empty strings which is fine for initial creation
	lcoreConfigMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.LCoreConfigCmName)
	llamaStackConfigMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.LlamaStackConfigCmName)

	// Get Proxy CA ConfigMap ResourceVersion if proxy is configured
	var proxyCACMResourceVersion string
	if cr.Spec.OLSConfig.ProxyConfig != nil {
		proxyCACertRef := cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef
		cmName := utils.GetProxyCACertConfigMapName(proxyCACertRef)
		if cmName != "" {
			proxyCACMResourceVersion, _ = utils.GetConfigMapResourceVersion(r, ctx, cmName)
		}
	}

	// Use helper functions to build common components
	labels := buildCommonLabels()

	// Build config volumes and mounts using helper functions
	llamaStackVolume, llamaStackConfigMount := buildLlamaStackConfigVolumeAndMount(&volumeDefaultMode)
	lcoreVolume, lcoreConfigMount := buildLCoreConfigVolumeAndMount(&volumeDefaultMode)

	// Define volumes
	volumes := []corev1.Volume{
		llamaStackVolume,
		lcoreVolume,
	}

	// Add PostgreSQL CA ConfigMap volume and mount (for TLS certificate verification)
	var postgresCAMounts []corev1.VolumeMount
	addPostgresCAVolumesAndMounts(&volumes, &postgresCAMounts, "/etc/certs/postgres-ca")

	// Add llama-cache EmptyDir for Llama Stack workspace
	var llamaCacheMounts []corev1.VolumeMount
	addLlamaCacheVolumesAndMounts(&volumes, &llamaCacheMounts)

	// Add TLS volumes and mounts (custom if provided, default otherwise)
	var tlsVolumeMounts []corev1.VolumeMount
	addCustomTLSVolumesAndMounts(&volumes, &tlsVolumeMounts, cr, &volumeDefaultMode)
	if len(tlsVolumeMounts) == 0 {
		// No custom TLS, add default service-ca TLS
		addTLSVolumesAndMounts(&volumes, &tlsVolumeMounts, cr, &volumeDefaultMode)
	}

	// Add OpenShift CA bundles (both service-ca and root CA)
	var openShiftCAMounts []corev1.VolumeMount
	addOpenShiftCAVolumesAndMounts(&volumes, &openShiftCAMounts, &volumeDefaultMode)
	addOpenShiftRootCAVolumesAndMounts(&volumes, &openShiftCAMounts, &volumeDefaultMode)

	// llama-stack container volume mounts
	llamaStackVolumeMounts := []corev1.VolumeMount{
		llamaStackConfigMount,
	}

	// Add PostgreSQL CA mount to llama-stack container
	llamaStackVolumeMounts = append(llamaStackVolumeMounts, postgresCAMounts...)

	// Add llama-cache mount to llama-stack container
	llamaStackVolumeMounts = append(llamaStackVolumeMounts, llamaCacheMounts...)

	// Add OpenShift CA mounts to llama-stack container
	llamaStackVolumeMounts = append(llamaStackVolumeMounts, openShiftCAMounts...)

	// Add user-provided CA certificates to llama-stack container
	addUserCAVolumesAndMounts(&volumes, &llamaStackVolumeMounts, cr, &volumeDefaultMode)

	// Proxy CA ConfigMap volume and mount (for proxy certificate verification)
	addProxyCACertVolumeAndMount(&volumes, &llamaStackVolumeMounts, cr, &volumeDefaultMode)

	// Build environment variables for LLM providers
	llamaStackEnvVars, err := buildLlamaStackEnvVars(r, ctx, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to build Llama Stack environment variables: %w", err)
	}

	llamaStackContainer := corev1.Container{
		Name:            "llama-stack",
		Image:           r.GetLCoreImage(),
		ImagePullPolicy: corev1.PullAlways,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8321,
				Name:          "llama-stack",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:          llamaStackEnvVars,
		VolumeMounts: llamaStackVolumeMounts,
		Command: []string{"bash", "-c", `
			# Start llama stack in background
			llama stack run run.yaml &
			LLAMA_PID=$!
			
			# Wait for llama stack to be healthy
			echo "Waiting for Llama Stack to start..."
			max_attempts=60
			attempt=0
			until curl -f http://localhost:8321/v1/health 2>/dev/null; do
				attempt=$((attempt + 1))
				if [ $attempt -ge $max_attempts ]; then
					echo "Llama Stack failed to start within timeout"
					exit 1
				fi
				sleep 2
			done
			echo "Llama Stack is healthy"
			
			# Warm up the embedding model (sentence-transformers)
			# This pre-loads the model used for RAG/vector search
			echo "Warming up embedding model (sentence-transformers)..."
			EMBEDDING_MODEL=$(grep -A 5 "model_type: embedding" /app-root/run.yaml | grep "model_id:" | head -1 | sed 's/.*model_id: *//' | tr -d ' ')
			if [ -n "$EMBEDDING_MODEL" ]; then
				echo "Using embedding model: $EMBEDDING_MODEL"
				curl -s -X POST http://localhost:8321/v1/inference/embeddings \
					-H "Content-Type: application/json" \
					-d "{\"model_id\": \"$EMBEDDING_MODEL\", \"contents\": [\"warmup\"]}" \
					> /dev/null 2>&1 && echo "Embedding warmup succeeded" || echo "Embedding warmup completed"
			fi
			
			# Warm up the safety model by making a test inference call
			# This forces Llama Guard to download and load into memory
			echo "Warming up safety model (Llama Guard via LLM inference)..."
			LLM_MODEL=$(grep -A 5 "model_type: llm" /app-root/run.yaml | grep "model_id:" | head -1 | sed 's/.*model_id: *//' | tr -d ' ')
			if [ -n "$LLM_MODEL" ]; then
				echo "Using LLM model: $LLM_MODEL"
				curl -s -X POST http://localhost:8321/v1/inference/chat-completion \
					-H "Content-Type: application/json" \
					-d "{\"model_id\": \"$LLM_MODEL\", \"messages\": [{\"role\": \"user\", \"content\": \"test\"}], \"stream\": false}" \
					> /dev/null 2>&1 && echo "LLM warmup succeeded" || echo "LLM warmup completed"
			else
				echo "No LLM model found in config, skipping LLM warmup"
			fi
			echo "Warmup complete, ready to serve traffic"
			
			# Keep running in foreground
			wait $LLAMA_PID
		`},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/v1/health",
					Port:   intstr.FromInt(8321),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 60, // Increased to account for model download + warmup
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/v1/health",
					Port:   intstr.FromInt(8321),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 60, // Increased to account for model download + warmup
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		Resources: *llamaStackResources,
	}

	// lightspeed-stack container
	lightspeedStackContainer := corev1.Container{
		Name:            "lightspeed-stack",
		Image:           r.GetLCoreImage(),
		ImagePullPolicy: corev1.PullAlways,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: utils.OLSAppServerContainerPort,
				Name:          "https",
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: buildLightspeedStackEnvVars(r, cr),
	}

	// Build lightspeed-stack volume mounts
	lightspeedStackVolumeMounts := []corev1.VolumeMount{
		lcoreConfigMount,
	}

	// Add TLS volume mounts from external secrets
	lightspeedStackVolumeMounts = append(lightspeedStackVolumeMounts, tlsVolumeMounts...)

	// PostgreSQL CA ConfigMap (service-ca.crt for OpenShift CA)
	lightspeedStackVolumeMounts = append(lightspeedStackVolumeMounts, utils.GetPostgresCAVolumeMount(path.Join(utils.OLSAppCertsMountRoot, "postgres-ca")))

	// Mount MCP server header secrets - only for HTTP-compatible servers
	if err := addMCPHeaderSecretVolumesAndMounts(r, ctx, &volumes, &lightspeedStackVolumeMounts, cr, &volumeDefaultMode); err != nil {
		return nil, err
	}

	// Add data collector volumes and mounts if enabled
	addDataCollectorVolumesAndMounts(&volumes, &lightspeedStackVolumeMounts, &volumeDefaultMode, dataCollectorEnabled)

	lightspeedStackContainer.VolumeMounts = lightspeedStackVolumeMounts
	lightspeedStackContainer.LivenessProbe = buildLightspeedStackLivenessProbe()
	lightspeedStackContainer.ReadinessProbe = buildLightspeedStackReadinessProbe()
	lightspeedStackContainer.Resources = *lightspeedStackResources

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.LCoreDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    labels,
			Annotations: map[string]string{
				utils.LCoreConfigMapResourceVersionAnnotation:      lcoreConfigMapResourceVersion,
				utils.LlamaStackConfigMapResourceVersionAnnotation: llamaStackConfigMapResourceVersion,
				utils.ProxyCACertHashKey:                           proxyCACMResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "lightspeed-stack",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: utils.OLSAppServerServiceAccountName,
					Containers: []corev1.Container{
						llamaStackContainer,
						lightspeedStackContainer,
					},
					Volumes: volumes,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}

	// Apply pod-level scheduling constraints (replicas configurable for lcore)
	utils.ApplyPodDeploymentConfig(&deployment, cr.Spec.OLSConfig.DeploymentConfig.APIContainer, true)

	if len(cr.Spec.OLSConfig.RAG) > 0 {
		if cr.Spec.OLSConfig.ImagePullSecrets != nil {
			deployment.Spec.Template.Spec.ImagePullSecrets = cr.Spec.OLSConfig.ImagePullSecrets
		}
	}

	if err := controllerutil.SetControllerReference(cr, &deployment, r.GetScheme()); err != nil {
		return nil, err
	}

	// Add OpenShift MCP server sidecar container if introspection is enabled
	addOpenShiftMCPServerSidecar(r, cr, &deployment)

	// Add data collector sidecar container if data collection is enabled
	addDataCollectorSidecar(r, cr, &deployment, lightspeedStackVolumeMounts, dataCollectorEnabled)

	return &deployment, nil
}

// RestartLCore triggers a rolling restart of the LCore deployment by updating its pod template annotation.
// This is useful when configuration changes require a pod restart (e.g., ConfigMap or Secret updates).
func RestartLCore(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	// If deployment is provided, use it; otherwise fetch it
	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		// Get the LCore deployment
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: utils.LCoreDeploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			r.GetLogger().Info("failed to get deployment", "deploymentName", utils.LCoreDeploymentName, "error", err)
			return err
		}
	}

	// Initialize annotations map if empty
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}

	// Bump the annotation to trigger a rolling update (new template hash)
	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	// Update the deployment
	r.GetLogger().Info("triggering LCore rolling restart", "deployment", dep.Name)
	err = r.Update(ctx, dep)
	if err != nil {
		r.GetLogger().Info("failed to update deployment", "deploymentName", dep.Name, "error", err)
		return err
	}

	return nil
}

// updateLCoreDeployment updates the LCore deployment based on CustomResource configuration
func updateLCoreDeployment(r reconciler.Reconciler, ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	// Step 1: Check if deployment spec has changed
	utils.SetDefaults_Deployment(desiredDeployment)
	changed := !utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec)

	// Step 2: Check ConfigMap ResourceVersions
	// Check if LCore ConfigMap ResourceVersion has changed
	currentLCoreConfigMapVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.LCoreConfigCmName)
	if err != nil {
		r.GetLogger().Info("failed to get LCore ConfigMap ResourceVersion", "error", err)
		changed = true
	} else {
		storedLCoreConfigMapVersion := existingDeployment.Annotations[utils.LCoreConfigMapResourceVersionAnnotation]
		if storedLCoreConfigMapVersion != currentLCoreConfigMapVersion {
			changed = true
		}
	}

	// Check if Llama Stack ConfigMap ResourceVersion has changed
	currentLlamaStackConfigMapVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.LlamaStackConfigCmName)
	if err != nil {
		r.GetLogger().Info("failed to get Llama Stack ConfigMap ResourceVersion", "error", err)
		changed = true
	} else {
		storedLlamaStackConfigMapVersion := existingDeployment.Annotations[utils.LlamaStackConfigMapResourceVersionAnnotation]
		if storedLlamaStackConfigMapVersion != currentLlamaStackConfigMapVersion {
			changed = true
		}
	}

	// Check if Proxy CA ConfigMap ResourceVersion has changed
	storedProxyCACMVersion := existingDeployment.Annotations[utils.ProxyCACertHashKey]
	currentProxyCACMVersion := desiredDeployment.Annotations[utils.ProxyCACertHashKey]
	if storedProxyCACMVersion != currentProxyCACMVersion {
		changed = true
	}

	// If nothing changed, skip update
	if !changed {
		return nil
	}

	// Apply changes - always update spec and annotations since something changed
	existingDeployment.Spec = desiredDeployment.Spec

	// Initialize annotations if nil
	if existingDeployment.Annotations == nil {
		existingDeployment.Annotations = make(map[string]string)
	}

	existingDeployment.Annotations[utils.LCoreConfigMapResourceVersionAnnotation] = desiredDeployment.Annotations[utils.LCoreConfigMapResourceVersionAnnotation]
	existingDeployment.Annotations[utils.LlamaStackConfigMapResourceVersionAnnotation] = desiredDeployment.Annotations[utils.LlamaStackConfigMapResourceVersionAnnotation]
	existingDeployment.Annotations[utils.ProxyCACertHashKey] = desiredDeployment.Annotations[utils.ProxyCACertHashKey]

	r.GetLogger().Info("updating LCore deployment", "name", existingDeployment.Name)

	if err := RestartLCore(r, ctx, existingDeployment); err != nil {
		return err
	}

	return nil
}

// generateLCoreLibraryDeployment generates the Deployment for LCore in library mode (single container)
func generateLCoreLibraryDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	ctx := context.Background()
	revisionHistoryLimit := int32(1)
	volumeDefaultMode := utils.VolumeDefaultMode

	// Check if data collector is enabled
	dataCollectorEnabled, err := dataCollectorEnabled(r, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to check data collector status: %w", err)
	}

	// Get ResourceVersions for tracking
	lcoreConfigMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.LCoreConfigCmName)
	llamaStackConfigMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.LlamaStackConfigCmName)

	// Get Proxy CA ConfigMap ResourceVersion if proxy is configured
	var proxyCACMResourceVersion string
	if cr.Spec.OLSConfig.ProxyConfig != nil {
		proxyCACertRef := cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef
		cmName := utils.GetProxyCACertConfigMapName(proxyCACertRef)
		if cmName != "" {
			proxyCACMResourceVersion, _ = utils.GetConfigMapResourceVersion(r, ctx, cmName)
		}
	}

	// Use helper functions to build common components
	labels := buildCommonLabels()

	// Build config volumes and mounts using helper functions
	llamaStackVolume, llamaStackConfigMount := buildLlamaStackConfigVolumeAndMount(&volumeDefaultMode)
	lcoreVolume, lcoreConfigMount := buildLCoreConfigVolumeAndMount(&volumeDefaultMode)

	// Library mode needs both volumes
	volumes := []corev1.Volume{
		llamaStackVolume,
		lcoreVolume,
	}

	// Library mode container needs both config mounts
	volumeMounts := []corev1.VolumeMount{
		llamaStackConfigMount,
		lcoreConfigMount,
	}

	// Add llama-cache EmptyDir for Llama Stack workspace
	addLlamaCacheVolumesAndMounts(&volumes, &volumeMounts)

	// Add TLS volumes and mounts (custom if provided, default otherwise)
	var tlsVolumeMounts []corev1.VolumeMount
	addCustomTLSVolumesAndMounts(&volumes, &tlsVolumeMounts, cr, &volumeDefaultMode)
	if len(tlsVolumeMounts) == 0 {
		// No custom TLS, add default service-ca TLS
		addTLSVolumesAndMounts(&volumes, &tlsVolumeMounts, cr, &volumeDefaultMode)
	}
	volumeMounts = append(volumeMounts, tlsVolumeMounts...)

	// Add OpenShift CA bundles (both service-ca and root CA)
	addOpenShiftCAVolumesAndMounts(&volumes, &volumeMounts, &volumeDefaultMode)
	addOpenShiftRootCAVolumesAndMounts(&volumes, &volumeMounts, &volumeDefaultMode)

	// Add PostgreSQL CA ConfigMap (for database TLS verification)
	addPostgresCAVolumesAndMounts(&volumes, &volumeMounts, "/etc/certs/postgres-ca")

	// Add user CA certificates
	addUserCAVolumesAndMounts(&volumes, &volumeMounts, cr, &volumeDefaultMode)

	// Proxy CA ConfigMap volume and mount (for proxy certificate verification)
	addProxyCACertVolumeAndMount(&volumes, &volumeMounts, cr, &volumeDefaultMode)

	// Add MCP header secrets for HTTP MCP servers
	if err := addMCPHeaderSecretVolumesAndMounts(r, ctx, &volumes, &volumeMounts, cr, &volumeDefaultMode); err != nil {
		return nil, err
	}

	// Add data collector volumes and mounts if enabled
	addDataCollectorVolumesAndMounts(&volumes, &volumeMounts, &volumeDefaultMode, dataCollectorEnabled)

	// Build environment variables for library mode
	// Library mode needs env vars from both llama-stack and lightspeed-stack
	llamaStackEnvVars, err := buildLlamaStackEnvVars(r, ctx, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to build llama-stack env vars: %w", err)
	}
	lightspeedStackEnvVars := buildLightspeedStackEnvVars(r, cr)

	// Combine env vars (llama-stack + lightspeed-stack)
	combinedEnvVars := append(llamaStackEnvVars, lightspeedStackEnvVars...)

	// Create the lightspeed-stack container using helper
	lightspeedStackContainer := buildLightspeedStackContainer(r, cr, volumeMounts, combinedEnvVars)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.LCoreDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    labels,
			Annotations: map[string]string{
				utils.LCoreConfigMapResourceVersionAnnotation:      lcoreConfigMapResourceVersion,
				utils.LlamaStackConfigMapResourceVersionAnnotation: llamaStackConfigMapResourceVersion,
				utils.ProxyCACertHashKey:                           proxyCACMResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "lightspeed-stack",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: utils.OLSAppServerServiceAccountName,
					Containers: []corev1.Container{
						lightspeedStackContainer,
					},
					Volumes: volumes,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}

	// Apply pod-level scheduling constraints
	utils.ApplyPodDeploymentConfig(&deployment, cr.Spec.OLSConfig.DeploymentConfig.APIContainer, true)

	if len(cr.Spec.OLSConfig.RAG) > 0 {
		if cr.Spec.OLSConfig.ImagePullSecrets != nil {
			deployment.Spec.Template.Spec.ImagePullSecrets = cr.Spec.OLSConfig.ImagePullSecrets
		}
	}

	if err := controllerutil.SetControllerReference(cr, &deployment, r.GetScheme()); err != nil {
		return nil, err
	}

	// Add OpenShift MCP server sidecar container if introspection is enabled
	addOpenShiftMCPServerSidecar(r, cr, &deployment)

	// Add data collector sidecar container if data collection is enabled
	addDataCollectorSidecar(r, cr, &deployment, volumeMounts, dataCollectorEnabled)

	return &deployment, nil
}

func dataCollectorEnabled(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (bool, error) {
	// Data collector is enabled when:
	// 1. User data collection is enabled in OLS configuration (feedback OR transcripts)
	// 2. AND telemetry is enabled (pull secret contains cloud.openshift.com auth)

	// Check if data collection is enabled in CR
	configEnabled := !cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled || !cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled
	if !configEnabled {
		return false, nil
	}

	// Check if telemetry is enabled
	// Telemetry enablement is determined by the presence of the telemetry pull secret
	// the presence of the field '.auths."cloud.openshift.com"' indicates that telemetry is enabled
	// use this command to check in an Openshift cluster:
	// oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' | jq '.auths."cloud.openshift.com"'
	pullSecret := &corev1.Secret{}
	err := r.Get(context.Background(), client.ObjectKey{Namespace: utils.TelemetryPullSecretNamespace, Name: utils.TelemetryPullSecretName}, pullSecret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	dockerconfigjson, ok := pullSecret.Data[".dockerconfigjson"]
	if !ok {
		return false, fmt.Errorf("pull secret does not contain .dockerconfigjson")
	}

	dockerconfigjsonDecoded := map[string]interface{}{}
	err = json.Unmarshal(dockerconfigjson, &dockerconfigjsonDecoded)
	if err != nil {
		return false, err
	}

	_, telemetryEnabled := dockerconfigjsonDecoded["auths"].(map[string]interface{})["cloud.openshift.com"]
	return telemetryEnabled, nil
}
