package lcore

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// GenerateLCoreDeployment generates the Deployment for LCore (llama-stack + lightspeed-stack)
func GenerateLCoreDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	ctx := context.Background()
	revisionHistoryLimit := int32(1)
	volumeDefaultMode := utils.VolumeDefaultMode

	llamaStackResources := getLlamaStackResources(cr)
	lightspeedStackResources := getLightspeedStackResources(cr)

	// Get ResourceVersions for tracking - these resources should already exist
	// If they don't exist, we'll get empty strings which is fine for initial creation
	lcoreConfigMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.LCoreConfigCmName)
	llamaStackConfigMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.LlamaStackConfigCmName)

	// Labels for the deployment
	labels := map[string]string{
		"app":                          "lightspeed-stack",
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}

	// Define volumes
	volumes := []corev1.Volume{
		{
			Name: "llama-stack-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: utils.LlamaStackConfigCmName,
					},
					DefaultMode: &volumeDefaultMode,
				},
			},
		},
		{
			Name: "lightspeed-stack-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: utils.LCoreConfigCmName,
					},
					DefaultMode: &volumeDefaultMode,
				},
			},
		},
		{
			Name: "llama-cache",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: utils.OpenShiftCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "kube-root-ca.crt",
					},
					DefaultMode: &volumeDefaultMode,
				},
			},
		},
	}

	// PostgreSQL CA ConfigMap volume (for TLS certificate verification)
	volumes = append(volumes, utils.GetPostgresCAConfigVolume())

	// Add external TLS secret if provided by user
	var tlsVolumeMounts []corev1.VolumeMount
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		// User provided custom TLS secret
		volumes = append(volumes, corev1.Volume{
			Name: "secret-lightspeed-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name,
					DefaultMode: &volumeDefaultMode,
				},
			},
		})

		tlsVolumeMounts = []corev1.VolumeMount{
			{
				Name:      "secret-lightspeed-tls",
				MountPath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls"),
				ReadOnly:  true,
			},
		}
	}

	// llama-stack container volume mounts
	llamaStackVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "llama-stack-config",
			MountPath: "/app-root/run.yaml",
			SubPath:   "run.yaml",
			ReadOnly:  true,
		},
		{
			Name:      "llama-cache",
			MountPath: "/app-root/.llama",
			ReadOnly:  false,
		},
		{
			Name:      utils.OpenShiftCAVolumeName,
			MountPath: "/etc/pki/ca-trust/extracted/pem",
			ReadOnly:  true,
		},
		// PostgreSQL CA ConfigMap (service-ca.crt for TLS verification)
		utils.GetPostgresCAVolumeMount("/etc/certs/postgres-ca"),
	}

	// User provided CA certificates - create both volumes and volume mounts in single pass
	_ = utils.ForEachExternalConfigMap(cr, func(name, source string) error {
		var volumeName, llamaStackMountPath string
		switch source {
		case "additional-ca":
			volumeName = utils.AdditionalCAVolumeName
			llamaStackMountPath = "/etc/pki/ca-trust/source/anchors"
		default:
			return nil
		}

		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: name},
					DefaultMode:          &volumeDefaultMode,
				},
			},
		})

		llamaStackVolumeMounts = append(llamaStackVolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: llamaStackMountPath,
			ReadOnly:  true,
		})
		return nil
	})

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
		{
			Name:      "lightspeed-stack-config",
			MountPath: "/app-root/lightspeed-stack.yaml",
			SubPath:   "lightspeed-stack.yaml",
		},
	}

	// Add TLS volume mounts from external secrets
	lightspeedStackVolumeMounts = append(lightspeedStackVolumeMounts, tlsVolumeMounts...)

	// TLS certificate volume and mount - only if using service-ca (not custom TLS)
	// If user provides TLSConfig.KeyCertSecretRef, it's already mounted above via ForEachExternalSecret
	if cr.Spec.OLSConfig.TLSConfig == nil || cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name == "" {
		volumes = append(volumes, corev1.Volume{
			Name: "secret-lightspeed-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.OLSCertsSecretName,
					DefaultMode: &volumeDefaultMode,
				},
			},
		})
		lightspeedStackVolumeMounts = append(lightspeedStackVolumeMounts, corev1.VolumeMount{
			Name:      "secret-lightspeed-tls",
			MountPath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls"),
			ReadOnly:  true,
		})
	}

	// PostgreSQL CA ConfigMap (service-ca.crt for OpenShift CA)
	lightspeedStackVolumeMounts = append(lightspeedStackVolumeMounts, utils.GetPostgresCAVolumeMount(path.Join(utils.OLSAppCertsMountRoot, "postgres-ca")))

	lightspeedStackContainer.VolumeMounts = lightspeedStackVolumeMounts
	lightspeedStackContainer.LivenessProbe = &corev1.Probe{
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
	lightspeedStackContainer.ReadinessProbe = &corev1.Probe{
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
	lightspeedStackContainer.Resources = *lightspeedStackResources

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lightspeed-stack-deployment",
			Namespace: r.GetNamespace(),
			Labels:    labels,
			Annotations: map[string]string{
				utils.LCoreConfigMapResourceVersionAnnotation:      lcoreConfigMapResourceVersion,
				utils.LlamaStackConfigMapResourceVersionAnnotation: llamaStackConfigMapResourceVersion,
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

	if err := controllerutil.SetControllerReference(cr, &deployment, r.GetScheme()); err != nil {
		return nil, err
	}

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

	// If nothing changed, skip update
	if !changed {
		return nil
	}

	// Apply changes - always update spec and annotations since something changed
	existingDeployment.Spec = desiredDeployment.Spec
	existingDeployment.Annotations[utils.LCoreConfigMapResourceVersionAnnotation] = desiredDeployment.Annotations[utils.LCoreConfigMapResourceVersionAnnotation]
	existingDeployment.Annotations[utils.LlamaStackConfigMapResourceVersionAnnotation] = desiredDeployment.Annotations[utils.LlamaStackConfigMapResourceVersionAnnotation]

	r.GetLogger().Info("updating LCore deployment", "name", existingDeployment.Name)

	if err := RestartLCore(r, ctx, existingDeployment); err != nil {
		return err
	}

	return nil
}
