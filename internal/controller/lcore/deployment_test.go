package lcore

import (
	"context"
	"testing"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// mockReconciler is a minimal mock for testing deployment generation
type mockReconciler struct {
	reconciler.Reconciler
	namespace string
	scheme    *runtime.Scheme
	image     string
}

func (m *mockReconciler) GetNamespace() string {
	if m.namespace == "" {
		return utils.OLSNamespaceDefault
	}
	return m.namespace
}

func (m *mockReconciler) GetScheme() *runtime.Scheme {
	if m.scheme == nil {
		scheme := runtime.NewScheme()
		_ = olsv1alpha1.AddToScheme(scheme)
		_ = appsv1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
		return scheme
	}
	return m.scheme
}

func (m *mockReconciler) GetAppServerImage() string {
	if m.image == "" {
		return utils.LlamaStackImageDefault
	}
	return m.image
}

func (m *mockReconciler) GetLCoreImage() string {
	if m.image == "" {
		return utils.LlamaStackImageDefault
	}
	return m.image
}

func (m *mockReconciler) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Return NotFound error for all Get calls in tests
	// This simulates the ConfigMaps not existing yet during deployment generation
	return errors.NewNotFound(schema.GroupResource{}, key.Name)
}

func TestGenerateLCoreDeployment(t *testing.T) {
	// Create a minimal OLSConfig CR
	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: "test-provider",
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			},
		},
	}

	// Create a mock reconciler
	r := &mockReconciler{}

	// Generate the deployment
	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Verify deployment is not nil
	if deployment == nil {
		t.Fatal("GenerateLCoreDeployment returned nil deployment")
	}

	// Verify basic metadata
	if deployment.Name != utils.LCoreDeploymentName {
		t.Errorf("Expected deployment name '%s', got '%s'", utils.LCoreDeploymentName, deployment.Name)
	}
	if deployment.Namespace != utils.OLSNamespaceDefault {
		t.Errorf("Expected namespace '%s', got '%s'", utils.OLSNamespaceDefault, deployment.Namespace)
	}

	// Verify labels
	expectedLabels := map[string]string{
		"app":                          utils.LCoreAppLabel,
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
	for key, expectedValue := range expectedLabels {
		if actualValue, ok := deployment.Labels[key]; !ok {
			t.Errorf("Missing label '%s'", key)
		} else if actualValue != expectedValue {
			t.Errorf("Label '%s': expected '%s', got '%s'", key, expectedValue, actualValue)
		}
	}

	// Verify replicas
	if deployment.Spec.Replicas == nil {
		t.Error("Replicas is nil")
	} else if *deployment.Spec.Replicas != 1 {
		t.Errorf("Expected 1 replica, got %d", *deployment.Spec.Replicas)
	}

	// Verify selector
	if deployment.Spec.Selector == nil {
		t.Fatal("Selector is nil")
	}
	if appLabel, ok := deployment.Spec.Selector.MatchLabels["app"]; !ok || appLabel != utils.LCoreAppLabel {
		t.Errorf("Expected selector matchLabel 'app: %s', got %v", utils.LCoreAppLabel, deployment.Spec.Selector.MatchLabels)
	}

	// Verify service account
	if deployment.Spec.Template.Spec.ServiceAccountName != utils.OLSAppServerServiceAccountName {
		t.Errorf("Expected ServiceAccountName '%s', got '%s'",
			utils.OLSAppServerServiceAccountName,
			deployment.Spec.Template.Spec.ServiceAccountName)
	}

	// Verify containers
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("Expected 2 containers, got %d", len(containers))
	}

	// Verify llama-stack container
	llamaStackContainer := containers[0]
	if llamaStackContainer.Name != utils.LlamaStackContainerName {
		t.Errorf("Expected first container name '%s', got '%s'", utils.LlamaStackContainerName, llamaStackContainer.Name)
	}
	if len(llamaStackContainer.Ports) != 1 || llamaStackContainer.Ports[0].ContainerPort != utils.LlamaStackContainerPort {
		t.Errorf("Expected llama-stack container port %d, got %v", utils.LlamaStackContainerPort, llamaStackContainer.Ports)
	}
	// Verify env vars are generated for all providers + POSTGRES_PASSWORD
	expectedEnvVars := len(cr.Spec.LLMConfig.Providers) + 1 // +1 for POSTGRES_PASSWORD
	if len(llamaStackContainer.Env) != expectedEnvVars {
		t.Errorf("Expected %d env vars (one per provider + POSTGRES_PASSWORD), got %d", expectedEnvVars, len(llamaStackContainer.Env))
	}
	// Check first provider's env var
	if len(llamaStackContainer.Env) > 0 {
		// Expected env var name using the ProviderNameToEnvVarName helper
		expectedEnvVarName := utils.ProviderNameToEnvVarName(cr.Spec.LLMConfig.Providers[0].Name) + "_API_KEY"
		if llamaStackContainer.Env[0].Name != expectedEnvVarName {
			t.Errorf("Expected env var '%s', got '%s'", expectedEnvVarName, llamaStackContainer.Env[0].Name)
		}
		if llamaStackContainer.Env[0].ValueFrom == nil || llamaStackContainer.Env[0].ValueFrom.SecretKeyRef == nil {
			t.Error("Expected env var to reference a secret")
		} else if llamaStackContainer.Env[0].ValueFrom.SecretKeyRef.Name != cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name {
			t.Errorf("Expected secret ref '%s', got '%s'",
				cr.Spec.LLMConfig.Providers[0].CredentialsSecretRef.Name,
				llamaStackContainer.Env[0].ValueFrom.SecretKeyRef.Name)
		}
	}
	if llamaStackContainer.LivenessProbe == nil || llamaStackContainer.LivenessProbe.HTTPGet == nil {
		t.Error("llama-stack container missing liveness probe")
	} else if llamaStackContainer.LivenessProbe.HTTPGet.Path != utils.LlamaStackHealthPath {
		t.Errorf("Expected liveness probe path '%s', got '%s'",
			utils.LlamaStackHealthPath, llamaStackContainer.LivenessProbe.HTTPGet.Path)
	}
	if llamaStackContainer.ReadinessProbe == nil || llamaStackContainer.ReadinessProbe.HTTPGet == nil {
		t.Error("llama-stack container missing readiness probe")
	}

	// Verify lightspeed-stack container
	lightspeedStackContainer := containers[1]
	if lightspeedStackContainer.Name != utils.LCoreContainerName {
		t.Errorf("Expected second container name '%s', got '%s'", utils.LCoreContainerName, lightspeedStackContainer.Name)
	}
	if len(lightspeedStackContainer.Ports) != 1 || lightspeedStackContainer.Ports[0].ContainerPort != utils.OLSAppServerContainerPort {
		t.Errorf("Expected lightspeed-stack container port %d, got %v",
			utils.OLSAppServerContainerPort, lightspeedStackContainer.Ports)
	}
	if lightspeedStackContainer.LivenessProbe == nil || lightspeedStackContainer.LivenessProbe.Exec == nil {
		t.Error("lightspeed-stack container missing liveness probe")
	}
	if lightspeedStackContainer.ReadinessProbe == nil || lightspeedStackContainer.ReadinessProbe.Exec == nil {
		t.Error("lightspeed-stack container missing readiness probe")
	}

	// Verify volumes
	volumes := deployment.Spec.Template.Spec.Volumes
	expectedVolumes := map[string]bool{
		utils.LlamaStackConfigCmName: false,
		utils.LCoreConfigCmName:      false,
		utils.LlamaCacheVolumeName:   false,
		"secret-lightspeed-tls":      false,
	}
	for _, vol := range volumes {
		if _, expected := expectedVolumes[vol.Name]; expected {
			expectedVolumes[vol.Name] = true
		}
	}
	for volName, found := range expectedVolumes {
		if !found {
			t.Errorf("Missing expected volume: %s", volName)
		}
	}

	// Verify volume mounts in llama-stack container
	llamaStackMounts := llamaStackContainer.VolumeMounts
	if len(llamaStackMounts) < 3 {
		t.Errorf("Expected at least 3 volume mounts in llama-stack container (config, cache, CA), got %d", len(llamaStackMounts))
	}
	llamaStackMountNames := make(map[string]bool)
	for _, mount := range llamaStackMounts {
		llamaStackMountNames[mount.Name] = true
	}
	if !llamaStackMountNames[utils.LlamaStackConfigCmName] {
		t.Errorf("Missing '%s' volume mount in llama-stack container", utils.LlamaStackConfigCmName)
	}
	if !llamaStackMountNames[utils.LlamaCacheVolumeName] {
		t.Errorf("Missing '%s' volume mount in llama-stack container", utils.LlamaCacheVolumeName)
	}
	if !llamaStackMountNames[utils.OpenShiftCAVolumeName] {
		t.Errorf("Missing '%s' volume mount in llama-stack container", utils.OpenShiftCAVolumeName)
	}

	// Verify volume mounts in lightspeed-stack container
	lightspeedStackMounts := lightspeedStackContainer.VolumeMounts
	if len(lightspeedStackMounts) != 3 {
		t.Errorf("Expected 3 volume mounts in lightspeed-stack container, got %d", len(lightspeedStackMounts))
	}
	lightspeedStackMountNames := make(map[string]bool)
	for _, mount := range lightspeedStackMounts {
		lightspeedStackMountNames[mount.Name] = true
	}
	if !lightspeedStackMountNames[utils.LCoreConfigCmName] {
		t.Errorf("Missing '%s' volume mount in lightspeed-stack container", utils.LCoreConfigCmName)
	}
	if !lightspeedStackMountNames["secret-lightspeed-tls"] {
		t.Error("Missing 'secret-lightspeed-tls' volume mount in lightspeed-stack container")
	}
	if !lightspeedStackMountNames[utils.PostgresCAVolume] {
		t.Errorf("Missing '%s' volume mount in lightspeed-stack container", utils.PostgresCAVolume)
	}

	// Verify that deployment can be marshaled to YAML (valid k8s object)
	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		t.Fatalf("Failed to marshal deployment to YAML: %v", err)
	}

	// Verify we can unmarshal it back
	var unmarshaledDeployment appsv1.Deployment
	err = yaml.Unmarshal(yamlBytes, &unmarshaledDeployment)
	if err != nil {
		t.Fatalf("Failed to unmarshal deployment YAML: %v", err)
	}

	t.Logf("Successfully validated LCore Deployment (%d bytes of YAML)", len(yamlBytes))
}

func TestGenerateLCoreDeploymentWithAdditionalCA(t *testing.T) {
	// Create an OLSConfig CR with additionalCAConfigMapRef
	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			LLMConfig: olsv1alpha1.LLMSpec{
				Providers: []olsv1alpha1.ProviderSpec{
					{
						Name: "test-provider",
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "test-secret",
						},
					},
				},
			},
			OLSConfig: olsv1alpha1.OLSSpec{
				AdditionalCAConfigMapRef: &corev1.LocalObjectReference{
					Name: "custom-ca-bundle",
				},
			},
		},
	}

	// Create a mock reconciler
	r := &mockReconciler{}

	// Generate the deployment
	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Verify deployment is not nil
	if deployment == nil {
		t.Fatal("GenerateLCoreDeployment returned nil deployment")
	}

	// Verify volumes include both kube-root-ca and additional CA
	volumes := deployment.Spec.Template.Spec.Volumes
	volumeNames := make(map[string]bool)
	for _, vol := range volumes {
		volumeNames[vol.Name] = true
	}

	// Should have kube-root-ca.crt
	if !volumeNames[utils.OpenShiftCAVolumeName] {
		t.Error("Missing 'kube-root-ca' volume")
	}

	// Should have additional CA volume
	if !volumeNames[utils.AdditionalCAVolumeName] {
		t.Error("Missing 'additional-ca' volume")
	}

	// Verify the additional CA volume is properly configured
	var additionalCAVolume *corev1.Volume
	for _, vol := range volumes {
		if vol.Name == utils.AdditionalCAVolumeName {
			additionalCAVolume = &vol
			break
		}
	}
	if additionalCAVolume == nil {
		t.Fatal("Additional CA volume not found")
	}
	if additionalCAVolume.ConfigMap == nil {
		t.Fatal("Additional CA volume is not a ConfigMap")
	}
	if additionalCAVolume.ConfigMap.Name != "custom-ca-bundle" {
		t.Errorf("Expected ConfigMap name 'custom-ca-bundle', got '%s'", additionalCAVolume.ConfigMap.Name)
	}

	// Verify llama-stack container has the additional CA volume mount
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("Expected 2 containers, got %d", len(containers))
	}

	llamaStackContainer := containers[0]
	if llamaStackContainer.Name != utils.LlamaStackContainerName {
		t.Fatalf("Expected first container to be '%s', got '%s'", utils.LlamaStackContainerName, llamaStackContainer.Name)
	}

	// Verify volume mounts include both kube-root-ca and additional CA
	volumeMounts := llamaStackContainer.VolumeMounts
	if len(volumeMounts) < 4 {
		t.Errorf("Expected at least 4 volume mounts (config, cache, kube-root-ca, additional-ca), got %d", len(volumeMounts))
	}

	volumeMountNames := make(map[string]string)
	for _, mount := range volumeMounts {
		volumeMountNames[mount.Name] = mount.MountPath
	}

	// Verify kube-root-ca mount
	if mountPath, ok := volumeMountNames[utils.OpenShiftCAVolumeName]; !ok {
		t.Errorf("Missing '%s' volume mount in llama-stack container", utils.OpenShiftCAVolumeName)
	} else if mountPath != utils.KubeRootCAMountPath {
		t.Errorf("Expected kube-root-ca mount path '%s', got '%s'", utils.KubeRootCAMountPath, mountPath)
	}

	// Verify additional CA mount
	if mountPath, ok := volumeMountNames[utils.AdditionalCAVolumeName]; !ok {
		t.Errorf("Missing '%s' volume mount in llama-stack container", utils.AdditionalCAVolumeName)
	} else if mountPath != utils.AdditionalCAMountPath {
		t.Errorf("Expected additional-ca mount path '%s', got '%s'", utils.AdditionalCAMountPath, mountPath)
	}

	// Verify all mounts are read-only
	for _, mount := range volumeMounts {
		if mount.Name == utils.OpenShiftCAVolumeName || mount.Name == utils.AdditionalCAVolumeName {
			if !mount.ReadOnly {
				t.Errorf("CA volume mount '%s' should be read-only", mount.Name)
			}
		}
	}

	t.Logf("Successfully validated LCore Deployment with Additional CA")
}
