package lcore

import (
	"context"
	"path"
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

// getLCoreReplicas returns the number of replicas for the LCore deployment
func getLCoreReplicas(cr *olsv1alpha1.OLSConfig) *int32 {
	// Use replicas from OLSConfig if specified, otherwise default to 1
	if cr.Spec.OLSConfig.DeploymentConfig.Replicas != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.Replicas
	}
	// default number of replicas.
	defaultReplicas := int32(1)
	return &defaultReplicas
}

// getLlamaStackResources returns resource requirements for the llama-stack container
// This container runs the Llama Stack inference service (sidecar to lightspeed-stack)
func getLlamaStackResources(_ *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	// llama-stack is a sidecar inference backend with fixed resource requirements
	// It always gets default resources to ensure stable inference performance
	// Users can configure the main API container (lightspeed-stack) via APIContainer.Resources
	// Note: The pod must have enough resources to accommodate both containers
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
	return defaultResources
}

// getLightspeedStackResources returns resource requirements for the lightspeed-stack container
// This is the main API container serving user requests
func getLightspeedStackResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	// Use custom resources from OLSConfig.DeploymentConfig.APIContainer if specified
	if cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Resources
	}

	// Default resources if not specified in CR
	defaultResources := &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("1Gi"),
			corev1.ResourceCPU:    resource.MustParse("1000m"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Claims: []corev1.ResourceClaim{},
	}
	return defaultResources
}

// buildLlamaStackEnvVars builds environment variables for all LLM providers
func buildLlamaStackEnvVars(cr *olsv1alpha1.OLSConfig) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	// Add environment variables for each provider
	for _, provider := range cr.Spec.LLMConfig.Providers {
		// Convert provider name to valid environment variable name
		envVarName := utils.ProviderNameToEnvVarName(provider.Name) + "_API_KEY"

		envVar := corev1.EnvVar{
			Name: envVarName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: provider.CredentialsSecretRef.Name,
					},
					Key: "apitoken",
				},
			},
		}
		envVars = append(envVars, envVar)
	}

	return envVars
}

// GenerateLCoreDeployment generates the Deployment for LCore (llama-stack + lightspeed-stack)
func GenerateLCoreDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	revisionHistoryLimit := int32(1)
	volumeDefaultMode := int32(420)

	replicas := getLCoreReplicas(cr)
	llamaStackResources := getLlamaStackResources(cr)
	lightspeedStackResources := getLightspeedStackResources(cr)

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
			Name: "secret-lightspeed-tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.OLSCertsSecretName,
					DefaultMode: &volumeDefaultMode,
				},
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

	// User provided additional CA certificates
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		volumes = append(volumes, corev1.Volume{
			Name: utils.AdditionalCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *cr.Spec.OLSConfig.AdditionalCAConfigMapRef,
					DefaultMode:          &volumeDefaultMode,
				},
			},
		})
	}

	// llama-stack container
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
	}

	// Mount additional CA if provided
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		llamaStackVolumeMounts = append(llamaStackVolumeMounts, corev1.VolumeMount{
			Name:      utils.AdditionalCAVolumeName,
			MountPath: "/etc/pki/ca-trust/source/anchors",
			ReadOnly:  true,
		})
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
		Env:          buildLlamaStackEnvVars(cr),
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
		Env: []corev1.EnvVar{
			{
				Name: "LOG_LEVEL",
				Value: func() string {
					if cr.Spec.OLSConfig.LogLevel != "" {
						return cr.Spec.OLSConfig.LogLevel
					}
					return "INFO"
				}(),
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "lightspeed-stack-config",
				MountPath: "/app-root/lightspeed-stack.yaml",
				SubPath:   "lightspeed-stack.yaml",
			},
			{
				Name:      "secret-lightspeed-tls",
				MountPath: path.Join(utils.OLSAppCertsMountRoot, "lightspeed-tls"),
				ReadOnly:  true,
			},
		},
		LivenessProbe: &corev1.Probe{
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
		},
		ReadinessProbe: &corev1.Probe{
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
		},
		Resources: *lightspeedStackResources,
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lightspeed-stack-deployment",
			Namespace: r.GetNamespace(),
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
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

	// Apply NodeSelector and Tolerations from APIContainer config if specified
	if cr.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = cr.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector
	}
	if cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations != nil {
		deployment.Spec.Template.Spec.Tolerations = cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations
	}

	if err := controllerutil.SetControllerReference(cr, &deployment, r.GetScheme()); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// RestartLCore triggers a rolling restart of the LCore deployment by updating its pod template annotation.
// This is useful when configuration changes require a pod restart (e.g., ConfigMap or Secret updates).
func RestartLCore(r reconciler.Reconciler, ctx context.Context) error {
	// Get the LCore deployment
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.LCoreDeploymentName, Namespace: r.GetNamespace()}, dep)
	if err != nil {
		r.GetLogger().Info("failed to get deployment", "deploymentName", utils.LCoreDeploymentName, "error", err)
		return err
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
