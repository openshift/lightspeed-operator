package lcore

import (
	"context"
	"fmt"
	"reflect"
	"strings"
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
	namespace       string
	scheme          *runtime.Scheme
	image           string
	lcoreServerMode bool
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

func (m *mockReconciler) GetOpenShiftMCPServerImage() string {
	return utils.OpenShiftMCPServerImageDefault
}

func (m *mockReconciler) GetLCoreServerMode() bool {
	return m.lcoreServerMode
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
	r := &mockReconciler{
		lcoreServerMode: true, // Test server mode (2 containers)
	}

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
	r := &mockReconciler{
		lcoreServerMode: true, // Test server mode (2 containers)
	}

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

func TestGenerateLCoreDeploymentWithIntrospection(t *testing.T) {
	// Create an OLSConfig CR with introspection enabled
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
				IntrospectionEnabled: true,
			},
		},
	}

	// Create a mock reconciler with OpenShift MCP server image
	r := &mockReconciler{
		lcoreServerMode: true, // Test server mode (2 containers + MCP sidecar)
	}

	// Generate the deployment
	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Verify deployment is not nil
	if deployment == nil {
		t.Fatal("GenerateLCoreDeployment returned nil deployment")
	}

	// Verify containers - should have 3: llama-stack, lightspeed-stack, openshift-mcp-server
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 3 {
		t.Fatalf("Expected 3 containers (llama-stack, lightspeed-stack, openshift-mcp-server), got %d", len(containers))
	}

	// Find the OpenShift MCP server container
	var openshiftMCPContainer *corev1.Container
	for i := range containers {
		if containers[i].Name == utils.OpenShiftMCPServerContainerName {
			openshiftMCPContainer = &containers[i]
			break
		}
	}

	if openshiftMCPContainer == nil {
		t.Fatal("OpenShift MCP server container not found in deployment")
	}

	// Verify container configuration
	if openshiftMCPContainer.Image == "" {
		t.Error("OpenShift MCP server container has empty image")
	}

	if openshiftMCPContainer.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("Expected ImagePullPolicy PullIfNotPresent, got %v", openshiftMCPContainer.ImagePullPolicy)
	}

	// Verify command includes port flag
	if len(openshiftMCPContainer.Command) == 0 {
		t.Error("OpenShift MCP server container has no command")
	} else {
		commandStr := strings.Join(openshiftMCPContainer.Command, " ")
		expectedPort := fmt.Sprintf("%d", utils.OpenShiftMCPServerPort)
		if !strings.Contains(commandStr, "--port") || !strings.Contains(commandStr, expectedPort) {
			t.Errorf("Expected command to include '--port %s', got: %s", expectedPort, commandStr)
		}
		if !strings.Contains(commandStr, "--read-only") {
			t.Error("Expected command to include '--read-only' flag")
		}
	}

	// Verify security context
	if openshiftMCPContainer.SecurityContext == nil {
		t.Error("OpenShift MCP server container has no security context")
	} else {
		if openshiftMCPContainer.SecurityContext.AllowPrivilegeEscalation == nil ||
			*openshiftMCPContainer.SecurityContext.AllowPrivilegeEscalation != false {
			t.Error("Expected AllowPrivilegeEscalation to be false")
		}
		if openshiftMCPContainer.SecurityContext.ReadOnlyRootFilesystem == nil ||
			*openshiftMCPContainer.SecurityContext.ReadOnlyRootFilesystem != true {
			t.Error("Expected ReadOnlyRootFilesystem to be true")
		}
	}

	// Verify resource requirements are set
	if openshiftMCPContainer.Resources.Limits == nil || openshiftMCPContainer.Resources.Requests == nil {
		t.Error("OpenShift MCP server container missing resource limits or requests")
	}

	t.Logf("Successfully validated LCore Deployment with OpenShift MCP server sidecar")
}

func TestGenerateLCoreDeploymentWithMCPHeaderSecrets(t *testing.T) {
	// Create an OLSConfig CR with MCP servers that use KUBERNETES_PLACEHOLDER only
	// (secrets will be validated in integration/e2e tests with real secrets)
	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: olsv1alpha1.OLSConfigSpec{
			FeatureGates: []olsv1alpha1.FeatureGate{utils.FeatureGateMCPServer},
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
			MCPServers: []olsv1alpha1.MCPServerConfig{
				{
					Name: "external-mcp-kubernetes-auth",
					URL:  "http://external1.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "Authorization",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeKubernetes,
							},
						},
					},
				},
				{
					Name: "external-mcp-mixed-auth",
					URL:  "https://external2.example.com",
					Headers: []olsv1alpha1.MCPHeader{
						{
							Name: "Authorization",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeKubernetes,
							},
						},
						{
							Name: "X-Custom",
							ValueFrom: olsv1alpha1.MCPHeaderValueSource{
								Type: olsv1alpha1.MCPHeaderSourceTypeKubernetes,
							},
						},
					},
				},
			},
		},
	}

	// Create a mock reconciler
	r := &mockReconciler{
		lcoreServerMode: true, // Test server mode (2 containers)
	}

	// Generate the deployment
	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Verify deployment is not nil
	if deployment == nil {
		t.Fatal("GenerateLCoreDeployment returned nil deployment")
	}

	// Verify volumes - should NOT have any MCP header secret volumes
	// since we only use KUBERNETES_PLACEHOLDER
	volumes := deployment.Spec.Template.Spec.Volumes
	volumeNames := make(map[string]bool)
	for _, vol := range volumes {
		volumeNames[vol.Name] = true
	}

	// Should NOT have header secret volumes (only using KUBERNETES_PLACEHOLDER)
	if volumeNames["header-mcp-auth-secret-1"] {
		t.Error("Should not have volume for KUBERNETES_PLACEHOLDER")
	}
	if volumeNames["header-"+utils.KUBERNETES_PLACEHOLDER] {
		t.Errorf("KUBERNETES_PLACEHOLDER should not create a volume, but found: header-%s", utils.KUBERNETES_PLACEHOLDER)
	}

	// Verify lightspeed-stack container has NO MCP header volume mounts
	containers := deployment.Spec.Template.Spec.Containers
	var lightspeedStackContainer *corev1.Container
	for i := range containers {
		if containers[i].Name == utils.LCoreContainerName {
			lightspeedStackContainer = &containers[i]
			break
		}
	}

	if lightspeedStackContainer == nil {
		t.Fatal("lightspeed-stack container not found")
	}

	// Check that no MCP header mounts exist (all use KUBERNETES_PLACEHOLDER)
	for _, mount := range lightspeedStackContainer.VolumeMounts {
		if strings.HasPrefix(mount.Name, "header-") {
			t.Errorf("Should not have MCP header volume mount, found: %s", mount.Name)
		}
	}

	t.Logf("Successfully validated LCore Deployment with KUBERNETES_PLACEHOLDER MCP headers (no secret volumes)")
}

func TestGenerateLCoreDeploymentWithoutIntrospection(t *testing.T) {
	// Create an OLSConfig CR with introspection disabled
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
				IntrospectionEnabled: false,
			},
		},
	}

	// Create a mock reconciler
	r := &mockReconciler{
		lcoreServerMode: true, // Test server mode (2 containers)
	}

	// Generate the deployment
	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Verify deployment is not nil
	if deployment == nil {
		t.Fatal("GenerateLCoreDeployment returned nil deployment")
	}

	// Verify containers - should have 2: llama-stack, lightspeed-stack (NO openshift-mcp-server)
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Fatalf("Expected 2 containers (llama-stack, lightspeed-stack), got %d", len(containers))
	}

	// Verify OpenShift MCP server container is NOT present
	for i := range containers {
		if containers[i].Name == utils.OpenShiftMCPServerContainerName {
			t.Error("OpenShift MCP server container should not be present when introspection is disabled")
		}
	}

	t.Logf("Successfully validated LCore Deployment without OpenShift MCP server sidecar")
}

func TestGetOLSMCPServerResources(t *testing.T) {
	cr := &olsv1alpha1.OLSConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}

	resources := getOLSMCPServerResources(cr)

	if resources == nil {
		t.Fatal("getOLSMCPServerResources returned nil")
	}

	// Verify limits
	if resources.Limits == nil {
		t.Error("Resources limits is nil")
	} else {
		memLimit := resources.Limits[corev1.ResourceMemory]
		if memLimit.IsZero() {
			t.Error("Memory limit is not set")
		}
		expectedMemLimit := "200Mi"
		if memLimit.String() != expectedMemLimit {
			t.Errorf("Expected memory limit '%s', got '%s'", expectedMemLimit, memLimit.String())
		}
	}

	// Verify requests
	if resources.Requests == nil {
		t.Error("Resources requests is nil")
	} else {
		cpuRequest := resources.Requests[corev1.ResourceCPU]
		memRequest := resources.Requests[corev1.ResourceMemory]

		if cpuRequest.IsZero() {
			t.Error("CPU request is not set")
		}
		if memRequest.IsZero() {
			t.Error("Memory request is not set")
		}

		expectedCPU := "50m"
		expectedMem := "64Mi"
		if cpuRequest.String() != expectedCPU {
			t.Errorf("Expected CPU request '%s', got '%s'", expectedCPU, cpuRequest.String())
		}
		if memRequest.String() != expectedMem {
			t.Errorf("Expected memory request '%s', got '%s'", expectedMem, memRequest.String())
		}
	}
}

func TestGenerateLCoreDeploymentLibraryMode(t *testing.T) {
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

	// Create a mock reconciler in library mode
	r := &mockReconciler{
		lcoreServerMode: false, // Test library mode (1 container)
	}

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
	if deployment.Name != "lightspeed-stack-deployment" {
		t.Errorf("Expected deployment name 'lightspeed-stack-deployment', got '%s'", deployment.Name)
	}
	if deployment.Namespace != utils.OLSNamespaceDefault {
		t.Errorf("Expected namespace '%s', got '%s'", utils.OLSNamespaceDefault, deployment.Namespace)
	}

	// Verify labels
	expectedLabels := map[string]string{
		"app":                          "lightspeed-stack",
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

	// Verify service account
	if deployment.Spec.Template.Spec.ServiceAccountName != utils.OLSAppServerServiceAccountName {
		t.Errorf("Expected ServiceAccountName '%s', got '%s'",
			utils.OLSAppServerServiceAccountName,
			deployment.Spec.Template.Spec.ServiceAccountName)
	}

	// Verify containers - should have ONLY 1 (lightspeed-stack with embedded llama-stack)
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 1 {
		t.Fatalf("Expected 1 container in library mode (lightspeed-stack), got %d", len(containers))
	}

	// Verify lightspeed-stack container
	lightspeedStackContainer := containers[0]
	if lightspeedStackContainer.Name != "lightspeed-service-api" {
		t.Errorf("Expected container name 'lightspeed-service-api', got '%s'", lightspeedStackContainer.Name)
	}
	if len(lightspeedStackContainer.Ports) != 1 || lightspeedStackContainer.Ports[0].ContainerPort != utils.OLSAppServerContainerPort {
		t.Errorf("Expected container port %d, got %v",
			utils.OLSAppServerContainerPort, lightspeedStackContainer.Ports)
	}
	if lightspeedStackContainer.LivenessProbe == nil {
		t.Error("lightspeed-stack container missing liveness probe")
	}
	if lightspeedStackContainer.ReadinessProbe == nil {
		t.Error("lightspeed-stack container missing readiness probe")
	}

	// Verify volumes - library mode needs BOTH config volumes (LCore + Llama Stack)
	volumes := deployment.Spec.Template.Spec.Volumes
	volumeNames := make(map[string]bool)
	for _, vol := range volumes {
		volumeNames[vol.Name] = true
	}

	// Both configs must be present
	if !volumeNames[utils.LCoreConfigCmName] {
		t.Error("Missing LCore config volume in library mode")
	}
	if !volumeNames[utils.LlamaStackConfigCmName] {
		t.Error("Missing Llama Stack config volume in library mode")
	}
	// Library mode also needs llama-cache for model downloads
	if !volumeNames[utils.LlamaCacheVolumeName] {
		t.Error("Missing llama-cache volume in library mode")
	}
	// Should have TLS
	if !volumeNames["secret-lightspeed-tls"] {
		t.Error("Missing TLS volume in library mode")
	}
	// Should have OpenShift root CA
	if !volumeNames[utils.OpenShiftCAVolumeName] {
		t.Error("Missing OpenShift root CA volume in library mode")
	}
	// Should have Postgres CA
	if !volumeNames[utils.PostgresCAVolume] {
		t.Error("Missing Postgres CA volume in library mode")
	}

	// Verify volume mounts in lightspeed-stack container
	volumeMounts := lightspeedStackContainer.VolumeMounts
	volumeMountNames := make(map[string]bool)
	for _, mount := range volumeMounts {
		volumeMountNames[mount.Name] = true
	}

	// Verify both configs are mounted
	if !volumeMountNames[utils.LCoreConfigCmName] {
		t.Error("Missing LCore config mount in library mode")
	}
	if !volumeMountNames[utils.LlamaStackConfigCmName] {
		t.Error("Missing Llama Stack config mount in library mode")
	}
	if !volumeMountNames[utils.LlamaCacheVolumeName] {
		t.Error("Missing llama-cache mount in library mode")
	}
	if !volumeMountNames["secret-lightspeed-tls"] {
		t.Error("Missing TLS mount in library mode")
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

	t.Logf("Successfully validated LCore Deployment in Library Mode (%d bytes of YAML)", len(yamlBytes))
}

func TestGenerateLCoreDeploymentWithRAG(t *testing.T) {
	imagePullSecrets := []corev1.LocalObjectReference{
		{
			Name: "byok-image-pull-secret-1",
		},
		{
			Name: "byok-image-pull-secret-2",
		},
	}

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
				ImagePullSecrets: imagePullSecrets,
				RAG: []olsv1alpha1.RAGSpec{
					{
						Image:     "byok-rag-image-1",
						IndexID:   "byok-index-id-1",
						IndexPath: "byok-index-path-1",
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

	if !reflect.DeepEqual(deployment.Spec.Template.Spec.ImagePullSecrets, imagePullSecrets) {
		t.Fatalf("Expected ImagePullSecrets: %+v, got %+v", imagePullSecrets, deployment.Spec.Template.Spec.ImagePullSecrets)
	}
}

func TestDataCollectorSidecar_Enabled(t *testing.T) {
	// Create CR with data collection enabled
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
				UserDataCollection: olsv1alpha1.UserDataCollectionSpec{
					FeedbackDisabled:    false,
					TranscriptsDisabled: false,
				},
			},
		},
	}

	// Create mock with telemetry enabled
	r := &mockReconcilerWithTelemetry{
		mockReconciler: mockReconciler{
			lcoreServerMode: true,
		},
		telemetryEnabled: true,
	}

	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Should have 3 containers: llama-stack, lightspeed-stack, data-collector
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 3 {
		t.Errorf("Expected 3 containers (llama-stack, lightspeed-stack, data-collector), got %d", len(containers))
	}

	// Verify data collector container exists
	var hasDataCollector bool
	for _, container := range containers {
		if container.Name == "lightspeed-to-dataverse-exporter" {
			hasDataCollector = true
			// Verify data-dir arg uses LCoreUserDataMountPath
			found := false
			for i, arg := range container.Args {
				if arg == "--data-dir" && i+1 < len(container.Args) {
					if container.Args[i+1] != utils.LCoreUserDataMountPath {
						t.Errorf("Expected data-dir %s, got %s", utils.LCoreUserDataMountPath, container.Args[i+1])
					}
					found = true
				}
			}
			if !found {
				t.Error("Data collector container missing --data-dir argument")
			}
		}
	}

	if !hasDataCollector {
		t.Error("Expected data collector sidecar container when data collection is enabled")
	}

	// Verify user data volume exists
	var hasUserDataVolume bool
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "ols-user-data" {
			hasUserDataVolume = true
			if volume.EmptyDir == nil {
				t.Error("Expected ols-user-data volume to be EmptyDir")
			}
		}
	}
	if !hasUserDataVolume {
		t.Error("Expected ols-user-data volume when data collection is enabled")
	}

	// Verify user data volume mount exists in lightspeed-stack container
	lightspeedStackContainer := containers[1] // Second container in server mode
	var hasUserDataMount bool
	for _, mount := range lightspeedStackContainer.VolumeMounts {
		if mount.Name == "ols-user-data" {
			hasUserDataMount = true
			if mount.MountPath != utils.LCoreUserDataMountPath {
				t.Errorf("Expected mount path %s, got %s", utils.LCoreUserDataMountPath, mount.MountPath)
			}
		}
	}
	if !hasUserDataMount {
		t.Error("Expected ols-user-data volume mount in lightspeed-stack container")
	}
}

func TestDataCollectorSidecar_Disabled(t *testing.T) {
	// Create CR with data collection disabled
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
				UserDataCollection: olsv1alpha1.UserDataCollectionSpec{
					FeedbackDisabled:    true,
					TranscriptsDisabled: true,
				},
			},
		},
	}

	r := &mockReconciler{
		lcoreServerMode: true,
	}

	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Should have 2 containers: llama-stack, lightspeed-stack (no data-collector)
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Errorf("Expected 2 containers (llama-stack, lightspeed-stack), got %d", len(containers))
	}

	// Verify data collector container does NOT exist
	for _, container := range containers {
		if container.Name == "lightspeed-to-dataverse-exporter" {
			t.Error("Data collector sidecar should not be present when data collection is disabled")
		}
	}

	// Verify user data volume does NOT exist
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "ols-user-data" {
			t.Error("ols-user-data volume should not be present when data collection is disabled")
		}
	}
}

func TestDataCollectorSidecar_LibraryMode(t *testing.T) {
	// Create CR with data collection enabled
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
				UserDataCollection: olsv1alpha1.UserDataCollectionSpec{
					FeedbackDisabled:    false,
					TranscriptsDisabled: false,
				},
			},
		},
	}

	// Create mock with library mode and telemetry enabled
	r := &mockReconcilerWithTelemetry{
		mockReconciler: mockReconciler{
			lcoreServerMode: false, // Library mode
		},
		telemetryEnabled: true,
	}

	deployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		t.Fatalf("GenerateLCoreDeployment returned error: %v", err)
	}

	// Should have 2 containers: lightspeed-stack, data-collector
	containers := deployment.Spec.Template.Spec.Containers
	if len(containers) != 2 {
		t.Errorf("Expected 2 containers (lightspeed-stack, data-collector), got %d", len(containers))
	}

	// Verify data collector container exists
	var hasDataCollector bool
	for _, container := range containers {
		if container.Name == "lightspeed-to-dataverse-exporter" {
			hasDataCollector = true
		}
	}

	if !hasDataCollector {
		t.Error("Expected data collector sidecar in library mode when data collection is enabled")
	}

	// Verify user data volume mount in single container
	lightspeedStackContainer := containers[0]
	var hasUserDataMount bool
	for _, mount := range lightspeedStackContainer.VolumeMounts {
		if mount.Name == "ols-user-data" && mount.MountPath == utils.LCoreUserDataMountPath {
			hasUserDataMount = true
		}
	}
	if !hasUserDataMount {
		t.Error("Expected ols-user-data volume mount in library mode")
	}
}

// mockReconcilerWithTelemetry extends mockReconciler to simulate telemetry pull secret
type mockReconcilerWithTelemetry struct {
	mockReconciler
	telemetryEnabled bool
}

func (m *mockReconcilerWithTelemetry) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	// Simulate telemetry pull secret lookup
	if key.Namespace == utils.TelemetryPullSecretNamespace && key.Name == utils.TelemetryPullSecretName {
		if m.telemetryEnabled {
			// Return a mock secret with cloud.openshift.com auth
			if secret, ok := obj.(*corev1.Secret); ok {
				secret.Data = map[string][]byte{
					".dockerconfigjson": []byte(`{"auths":{"cloud.openshift.com":{}}}`),
				}
				return nil
			}
		}
		// Telemetry disabled - return not found
		return errors.NewNotFound(schema.GroupResource{}, key.Name)
	}

	// For other resources, use the parent mock behavior
	return m.mockReconciler.Get(ctx, key, obj, opts...)
}

func (m *mockReconcilerWithTelemetry) GetDataverseExporterImage() string {
	return utils.DataverseExporterImageDefault
}
