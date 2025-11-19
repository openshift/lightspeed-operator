// Package utils provides shared utility functions, types, and constants used across
// the OpenShift Lightspeed operator components.
//
// This package contains:
//   - Constants for resource names, labels, and annotations
//   - Error constants for consistent error handling
//   - Helper functions for Kubernetes resource operations
//   - Status condition utilities
//   - TLS certificate validation
//   - OpenShift version detection
//   - Configuration data structures for OLS components
//
// The utilities in this package are designed to be reusable across all operator
// components (appserver, postgres, console) and promote consistency in resource
// naming, labeling, and error handling throughout the codebase.
package utils

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

// setDeploymentContainerEnvs sets the envs for a specific container in a given deployment.
func SetDeploymentContainerEnvs(deployment *appsv1.Deployment, desiredEnvs []corev1.EnvVar, containerName string) (bool, error) {
	containerIndex, err := GetContainerIndex(deployment, containerName)
	if err != nil {
		return false, err
	}
	existingEnvs := deployment.Spec.Template.Spec.Containers[containerIndex].Env
	if !apiequality.Semantic.DeepEqual(existingEnvs, desiredEnvs) {
		deployment.Spec.Template.Spec.Containers[containerIndex].Env = desiredEnvs
		return true, nil
	}
	return false, nil
}

// setDeploymentContainerResources sets the resource requirements for a specific container in a given deployment.
// setDeploymentContainerVolumeMounts sets the volume mounts for a specific container in a given deployment.
func SetDeploymentContainerVolumeMounts(deployment *appsv1.Deployment, containerName string, volumeMounts []corev1.VolumeMount) (bool, error) {
	containerIndex, err := GetContainerIndex(deployment, containerName)
	if err != nil {
		return false, err
	}
	existingVolumeMounts := deployment.Spec.Template.Spec.Containers[containerIndex].VolumeMounts
	if !apiequality.Semantic.DeepEqual(existingVolumeMounts, volumeMounts) {
		deployment.Spec.Template.Spec.Containers[containerIndex].VolumeMounts = volumeMounts
		return true, nil
	}

	return false, nil
}

// getContainerIndex returns the index of the container with the specified name in a given deployment.
func GetContainerIndex(deployment *appsv1.Deployment, containerName string) (int, error) {
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			return i, nil
		}
	}
	return -1, fmt.Errorf("container %s not found in deployment %s", containerName, deployment.Name)
}

// ProviderNameToEnvVarName converts a provider name to a valid environment variable name.
// Kubernetes resource names typically use hyphens (DNS-1123), but environment variable
// names cannot contain hyphens. This function replaces hyphens with underscores and
// converts to uppercase for consistency with environment variable naming conventions.
//
// Example: "my-provider" -> "MY_PROVIDER"
func ProviderNameToEnvVarName(providerName string) string {
	// Replace hyphens with underscores for valid environment variable names
	envVarName := strings.ReplaceAll(providerName, "-", "_")
	// Convert to uppercase for standard environment variable convention
	return strings.ToUpper(envVarName)
}

func GetSecretContent(rclient client.Client, secretName string, namespace string, secretFields []string, foundSecret *corev1.Secret) (map[string]string, error) {
	ctx := context.Background()
	err := rclient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, foundSecret)
	if err != nil {
		return nil, fmt.Errorf("secret not found: %s. error: %w", secretName, err)
	}
	secretValues := make(map[string]string)
	for _, field := range secretFields {
		value, ok := foundSecret.Data[field]
		if !ok {
			return nil, fmt.Errorf("secret field %s not present in the secret", field)
		}
		secretValues[field] = string(value)
	}

	return secretValues, nil
}

func GetAllSecretContent(rclient client.Client, secretName string, namespace string, foundSecret *corev1.Secret) (map[string]string, error) {
	ctx := context.Background()
	err := rclient.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, foundSecret)
	if err != nil {
		return nil, fmt.Errorf("secret not found: %s. error: %w", secretName, err)
	}

	secretValues := make(map[string]string)
	for key, value := range foundSecret.Data {
		secretValues[key] = string(value)
	}

	return secretValues, nil
}

// podVolumEqual compares two slices of corev1.Volume and returns true if they are equal.
// covers 3 volume types: Secret, ConfigMap, EmptyDir
func PodVolumeEqual(a, b []corev1.Volume) bool {
	if len(a) != len(b) {
		return false
	}
	aVolumeMap := make(map[string]corev1.Volume)
	for _, v := range a {
		aVolumeMap[v.Name] = v
	}
	bVolumeMap := make(map[string]corev1.Volume)
	for _, v := range b {
		bVolumeMap[v.Name] = v
	}
	for name, aVolume := range aVolumeMap {
		if bVolume, exist := bVolumeMap[name]; exist {
			if aVolume.Secret != nil && bVolume.Secret != nil {
				if aVolume.Secret.SecretName != bVolume.Secret.SecretName {
					return false
				}
				continue
			}
			if aVolume.ConfigMap != nil && bVolume.ConfigMap != nil {
				if aVolume.ConfigMap.Name != bVolume.ConfigMap.Name {
					return false
				}
				continue
			}
			if aVolume.EmptyDir != nil && bVolume.EmptyDir != nil {
				if aVolume.EmptyDir.Medium != bVolume.EmptyDir.Medium {
					return false
				}
				continue
			}
			if aVolume.PersistentVolumeClaim != nil && bVolume.PersistentVolumeClaim != nil {
				if aVolume.PersistentVolumeClaim.ClaimName != bVolume.PersistentVolumeClaim.ClaimName {
					return false
				}
				continue
			}

			return false
		}
		return false
	}

	return true
}

// deploymentSpecEqual compares two appsv1.DeploymentSpec and returns true if they are equal.
// ConfigMapEqual compares two ConfigMaps for equality, checking Data and BinaryData
func ConfigMapEqual(a, b *corev1.ConfigMap) bool {
	return apiequality.Semantic.DeepEqual(a.Data, b.Data) &&
		apiequality.Semantic.DeepEqual(a.BinaryData, b.BinaryData)
}

func DeploymentSpecEqual(a, b *appsv1.DeploymentSpec) bool {
	if !apiequality.Semantic.DeepEqual(a.Template.Spec.NodeSelector, b.Template.Spec.NodeSelector) || // check node selector
		!apiequality.Semantic.DeepEqual(a.Template.Spec.Tolerations, b.Template.Spec.Tolerations) || // check toleration
		!apiequality.Semantic.DeepEqual(a.Strategy, b.Strategy) || // check strategy
		!PodVolumeEqual(a.Template.Spec.Volumes, b.Template.Spec.Volumes) || // check volumes
		*a.Replicas != *b.Replicas { // check replicas
		return false
	}

	// check containers
	if !ContainersEqual(a.Template.Spec.Containers, b.Template.Spec.Containers) {
		return false
	}

	// check init containers
	if !ContainersEqual(a.Template.Spec.InitContainers, b.Template.Spec.InitContainers) {
		return false
	}

	return true
}

// containerEqual compares two container arrays and returns true if they are equal.
func ContainersEqual(a, b []corev1.Container) bool {
	// check containers
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !ContainerSpecEqual(&a[i], &b[i]) {
			return false
		}
	}
	return true
}

// containerSpecEqual compares two corev1.Container and returns true if they are equal.
// checks performed on limited fields
func ContainerSpecEqual(a, b *corev1.Container) bool {
	return (a.Name == b.Name && // check name
		a.Image == b.Image && // check image
		apiequality.Semantic.DeepEqual(a.Ports, b.Ports) && // check ports
		EnvEqual(a.Env, b.Env) && // check env (order-insensitive)
		apiequality.Semantic.DeepEqual(a.Args, b.Args) && // check arguments
		VolumeMountsEqual(a.VolumeMounts, b.VolumeMounts) && // check volume mounts (order-insensitive)
		apiequality.Semantic.DeepEqual(a.Resources, b.Resources) && // check resources
		apiequality.Semantic.DeepEqual(a.SecurityContext, b.SecurityContext) && // check security context
		a.ImagePullPolicy == b.ImagePullPolicy && // check image pull policy
		ProbeEqual(a.LivenessProbe, b.LivenessProbe) && // check liveness probe
		ProbeEqual(a.ReadinessProbe, b.ReadinessProbe) && // check readiness probe
		ProbeEqual(a.StartupProbe, b.StartupProbe)) // check startup probe
}

// EnvEqual compares two EnvVar slices ignoring order
func EnvEqual(a, b []corev1.EnvVar) bool {
	if len(a) != len(b) {
		return false
	}
	aEnvMap := make(map[string]corev1.EnvVar)
	for _, env := range a {
		aEnvMap[env.Name] = env
	}
	bEnvMap := make(map[string]corev1.EnvVar)
	for _, env := range b {
		bEnvMap[env.Name] = env
	}
	for name, aEnv := range aEnvMap {
		bEnv, exist := bEnvMap[name]
		if !exist {
			return false
		}
		if !apiequality.Semantic.DeepEqual(aEnv, bEnv) {
			return false
		}
	}
	return true
}

// VolumeMountsEqual compares two VolumeMount slices ignoring order
func VolumeMountsEqual(a, b []corev1.VolumeMount) bool {
	if len(a) != len(b) {
		return false
	}
	aVolumeMountMap := make(map[string]corev1.VolumeMount)
	for _, vm := range a {
		aVolumeMountMap[vm.Name] = vm
	}
	bVolumeMountMap := make(map[string]corev1.VolumeMount)
	for _, vm := range b {
		bVolumeMountMap[vm.Name] = vm
	}
	for name, aVolumeMount := range aVolumeMountMap {
		bVolumeMount, exist := bVolumeMountMap[name]
		if !exist {
			return false
		}
		if !apiequality.Semantic.DeepEqual(aVolumeMount, bVolumeMount) {
			return false
		}
	}
	return true
}

func ProbeEqual(a, b *corev1.Probe) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if !apiequality.Semantic.DeepEqual(a.ProbeHandler, b.ProbeHandler) {
		return false
	}

	arrA := []int32{a.InitialDelaySeconds, a.TimeoutSeconds, a.PeriodSeconds, a.SuccessThreshold, a.FailureThreshold}
	arrB := []int32{b.InitialDelaySeconds, b.TimeoutSeconds, b.PeriodSeconds, b.SuccessThreshold, b.FailureThreshold}
	for i := range arrA {
		// unset values are considered equal
		if arrA[i] == 0 || arrB[i] == 0 {
			continue
		}
		if arrA[i] != arrB[i] {
			return false
		}
	}

	return apiequality.Semantic.DeepEqual(a.TerminationGracePeriodSeconds, b.TerminationGracePeriodSeconds)
}

// serviceEqual compares two v1.Service and returns true if they are equal.
func ServiceEqual(a *corev1.Service, b *corev1.Service) bool {
	if !apiequality.Semantic.DeepEqual(a.Labels, b.Labels) ||
		!apiequality.Semantic.DeepEqual(a.Spec.Selector, b.Spec.Selector) ||
		len(a.Spec.Ports) != len(b.Spec.Ports) {
		return false
	}

	for i, aPort := range a.Spec.Ports {
		bPort := b.Spec.Ports[i]
		if !apiequality.Semantic.DeepEqual(aPort, bPort) {
			return false
		}
	}

	return true
}

// serviceMonitorEqual compares two monv1.ServiceMonitor and returns true if they are equal.
func ServiceMonitorEqual(a *monv1.ServiceMonitor, b *monv1.ServiceMonitor) bool {
	return apiequality.Semantic.DeepEqual(a.Labels, b.Labels) &&
		apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
}

// prometheusRuleEqual compares two monv1.PrometheusRule and returns true if they are equal.
func PrometheusRuleEqual(a *monv1.PrometheusRule, b *monv1.PrometheusRule) bool {
	return apiequality.Semantic.DeepEqual(a.Labels, b.Labels) &&
		apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
}

// networkPolicyEqual compares two networkingv1.NetworkPolicy and returns true if they are equal.
func NetworkPolicyEqual(a *networkingv1.NetworkPolicy, b *networkingv1.NetworkPolicy) bool {
	return apiequality.Semantic.DeepEqual(a.Labels, b.Labels) &&
		apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
}

// This is copied from https://github.com/kubernetes/kubernetes/blob/v1.29.2/pkg/apis/apps/v1/defaults.go#L38
// to avoid importing the whole k8s.io/kubernetes package.
// SetDefaults_Deployment sets additional defaults compared to its counterpart
// in extensions. These addons are:
// - MaxUnavailable during rolling update set to 25% (1 in extensions)
// - MaxSurge value during rolling update set to 25% (1 in extensions)
// - RevisionHistoryLimit set to 10 (not set in extensions)
// - ProgressDeadlineSeconds set to 600s (not set in extensions)
func SetDefaults_Deployment(obj *appsv1.Deployment) {
	// Set DeploymentSpec.Replicas to 1 if it is not set.
	if obj.Spec.Replicas == nil {
		obj.Spec.Replicas = new(int32)
		*obj.Spec.Replicas = 1
	}
	strategy := &obj.Spec.Strategy
	// Set default DeploymentStrategyType as RollingUpdate.
	if strategy.Type == "" {
		strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	}
	if strategy.Type == appsv1.RollingUpdateDeploymentStrategyType {
		if strategy.RollingUpdate == nil {
			rollingUpdate := appsv1.RollingUpdateDeployment{}
			strategy.RollingUpdate = &rollingUpdate
		}
		if strategy.RollingUpdate.MaxUnavailable == nil {
			// Set default MaxUnavailable as 25% by default.
			maxUnavailable := intstr.FromString("25%")
			strategy.RollingUpdate.MaxUnavailable = &maxUnavailable
		}
		if strategy.RollingUpdate.MaxSurge == nil {
			// Set default MaxSurge as 25% by default.
			maxSurge := intstr.FromString("25%")
			strategy.RollingUpdate.MaxSurge = &maxSurge
		}
	}
	if obj.Spec.RevisionHistoryLimit == nil {
		obj.Spec.RevisionHistoryLimit = new(int32)
		*obj.Spec.RevisionHistoryLimit = 10
	}
	if obj.Spec.ProgressDeadlineSeconds == nil {
		obj.Spec.ProgressDeadlineSeconds = new(int32)
		*obj.Spec.ProgressDeadlineSeconds = 600
	}
}

func GetProxyEnvVars() []corev1.EnvVar {
	envVars := []corev1.EnvVar{}
	for _, envvar := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
		if value := os.Getenv(envvar); value != "" {
			envVars = append(envVars, corev1.EnvVar{
				Name:  strings.ToLower(envvar),
				Value: value,
			})
		}
	}
	return envVars
}

// validate the x509 certificate syntax
func ValidateCertificateFormat(cert []byte) error {
	if len(cert) == 0 {
		return fmt.Errorf("certificate is empty")
	}
	block, _ := pem.Decode(cert)
	if block == nil {
		return fmt.Errorf("failed to decode PEM certificate")
	}
	if block.Type != "CERTIFICATE" {
		return fmt.Errorf("block type is not certificate but %s", block.Type)
	}
	// check the CA is correctly formatted
	_, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %v", err)
	}

	return nil
}

// Get Openshift version
func GetOpenshiftVersion(k8sClient client.Client, ctx context.Context) (string, string, error) {
	key := client.ObjectKey{Name: "version"}
	clusterVersion := &configv1.ClusterVersion{}
	if err := k8sClient.Get(ctx, key, clusterVersion); err != nil {
		return "", "", err
	}
	openshift_versions := strings.Split(clusterVersion.Status.Desired.Version, ".")
	if len(openshift_versions) < 2 {
		return "", "", fmt.Errorf("failed to parse cluster version: %s", clusterVersion.Status.Desired.Version)
	}
	return openshift_versions[0], openshift_versions[1], nil
}

// GeneratePostgresSelectorLabels returns selector labels for Postgres components
func GeneratePostgresSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "postgres-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-postgres",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

// GenerateAppServerSelectorLabels returns selector labels for Application Server components
func GenerateAppServerSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "application-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-api",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

// AnnotateSecretWatcher adds the watcher annotation to a secret
func AnnotateSecretWatcher(secret *corev1.Secret) {
	annotations := secret.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[WatcherAnnotationKey] = OLSConfigName
	secret.SetAnnotations(annotations)
}

// AnnotateConfigMapWatcher adds the watcher annotation to a configmap
func AnnotateConfigMapWatcher(cm *corev1.ConfigMap) {
	annotations := cm.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[WatcherAnnotationKey] = OLSConfigName
	cm.SetAnnotations(annotations)
}

// IsPrometheusOperatorAvailable checks if Prometheus Operator CRDs are available on the cluster.
// It attempts to list ServiceMonitor and PrometheusRule resources to determine availability.
// Returns true if both CRDs are present, false otherwise.
func IsPrometheusOperatorAvailable(ctx context.Context, c client.Client) bool {
	// Check ServiceMonitor CRD
	serviceMonitorList := &monv1.ServiceMonitorList{}
	if err := c.List(ctx, serviceMonitorList, &client.ListOptions{Limit: 1}); err != nil {
		return false
	}

	// Check PrometheusRule CRD
	prometheusRuleList := &monv1.PrometheusRuleList{}
	if err := c.List(ctx, prometheusRuleList, &client.ListOptions{Limit: 1}); err != nil {
		return false
	}

	return true
}

// ValidateExternalSecrets validates that all external secrets referenced in the CR exist and are accessible.
// This includes LLM provider credentials and MCP server headers.
// It also annotates the secrets for watching.
func ValidateExternalSecrets(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	// Validate LLM provider credentials
	for _, provider := range cr.Spec.LLMConfig.Providers {
		foundSecret := &corev1.Secret{}
		_, err := GetAllSecretContent(r, provider.CredentialsSecretRef.Name, r.GetNamespace(), foundSecret)
		if err != nil {
			return fmt.Errorf("LLM provider credential secret not found for provider %s: %w", provider.Name, err)
		}
		AnnotateSecretWatcher(foundSecret)
		err = r.Update(ctx, foundSecret)
		if err != nil {
			return fmt.Errorf("failed to update LLM provider secret %s: %w", foundSecret.Name, err)
		}
	}

	// Validate MCP server headers (if any)
	if cr.Spec.MCPServers != nil {
		for _, mcpServer := range cr.Spec.MCPServers {
			if mcpServer.StreamableHTTP != nil && mcpServer.StreamableHTTP.Headers != nil {
				for headerName, secretName := range mcpServer.StreamableHTTP.Headers {
					// Skip the special "kubernetes" token case
					if secretName == KUBERNETES_PLACEHOLDER {
						continue
					}
					foundSecret := &corev1.Secret{}
					_, err := GetAllSecretContent(r, secretName, r.GetNamespace(), foundSecret)
					if err != nil {
						return fmt.Errorf("MCP server header secret not found for server %s, header %s: %w", mcpServer.Name, headerName, err)
					}
					AnnotateSecretWatcher(foundSecret)
					err = r.Update(ctx, foundSecret)
					if err != nil {
						return fmt.Errorf("failed to update MCP server header secret %s: %w", foundSecret.Name, err)
					}
				}
			}
		}
	}

	return nil
}

// GetConfigMapResourceVersion returns the ResourceVersion of a ConfigMap.
func GetConfigMapResourceVersion(r reconciler.Reconciler, ctx context.Context, configMapName string) (string, error) {
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: r.GetNamespace()}, configMap)
	if err != nil {
		return "", err
	}
	return configMap.ResourceVersion, nil
}

// GetSecretResourceVersion returns the ResourceVersion of a Secret.
func GetSecretResourceVersion(r reconciler.Reconciler, ctx context.Context, secretName string) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: r.GetNamespace()}, secret)
	if err != nil {
		return "", err
	}
	return secret.ResourceVersion, nil
}
