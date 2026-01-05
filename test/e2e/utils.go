package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// OLSTestEnvironment contains all the resources needed for TLS testing
type OLSTestEnvironment struct {
	Client       *Client
	CR           *olsv1alpha1.OLSConfig
	SAToken      string
	ForwardHost  string
	CleanUpFuncs []func()
}

// SetupOLSTestEnvironment sets up the common test environment for TLS tests
func SetupOLSTestEnvironment(crModifier func(*olsv1alpha1.OLSConfig)) (*OLSTestEnvironment, error) {
	env := &OLSTestEnvironment{
		CleanUpFuncs: make([]func(), 0),
	}

	var err error
	env.Client, err = GetClient(nil)
	if err != nil {
		return nil, err
	}

	// Create OLSConfig CR
	env.CR, err = generateOLSConfig()
	if err != nil {
		return nil, err
	}

	// Apply any modifications to the CR
	if crModifier != nil {
		crModifier(env.CR)
	}

	err = env.Client.Create(env.CR)
	if err != nil {
		return nil, err
	}

	// Create service account for OLS user
	cleanUp, err := env.Client.CreateServiceAccount(OLSNameSpace, TestSAName)
	if err != nil {
		return nil, err
	}
	env.CleanUpFuncs = append(env.CleanUpFuncs, cleanUp)

	// Create role binding for OLS user accessing query API
	cleanUp, err = env.Client.CreateClusterRoleBinding(OLSNameSpace, TestSAName, QueryAccessClusterRole)
	if err != nil {
		return nil, err
	}
	env.CleanUpFuncs = append(env.CleanUpFuncs, cleanUp)

	// Fetch the service account token
	env.SAToken, err = env.Client.GetServiceAccountToken(OLSNameSpace, TestSAName)
	if err != nil {
		return nil, err
	}

	// Wait for application server deployment rollout
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerDeploymentName,
			Namespace: OLSNameSpace,
		},
	}
	err = env.Client.WaitForDeploymentRollout(deployment)
	if err != nil {
		return nil, err
	}

	// Forward the HTTPS port to a local port
	env.ForwardHost, cleanUp, err = env.Client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
	if err != nil {
		return nil, err
	}
	env.CleanUpFuncs = append(env.CleanUpFuncs, cleanUp)

	return env, nil
}

func CheckErrorAndRestartPortForwardingTestEnvironment(env *OLSTestEnvironment, err error) {
	if err == nil {
		return
	}
	if !strings.Contains(err.Error(), "EOF") {
		return
	}
	fmt.Printf("EOF error detected, restarting port forwarding\n")
	errPf := RestartPortForwardingTestEnvironment(env)
	if errPf != nil {
		fmt.Printf("failed to restart port forwarding: %s \n", errPf)
	}
}

func RestartPortForwardingTestEnvironment(env *OLSTestEnvironment) error {
	var cleanUp func()
	var err error
	// Wait for application server deployment have a ready pod
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerDeploymentName,
			Namespace: OLSNameSpace,
		},
	}
	err = env.Client.WaitForDeploymentRollout(deployment)
	if err != nil {
		fmt.Printf("port forwarding not restarted, failed to wait for deployment rollout: %s \n", err)
		return err
	}
	env.ForwardHost, cleanUp, err = env.Client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
	if err != nil {
		fmt.Printf("failed to restart port forwarding: %s \n", err)
		return err
	}
	env.CleanUpFuncs = append(env.CleanUpFuncs, cleanUp)
	return nil
}

// TestOLSServiceActivation tests that TLS is properly activated on the service
func TestOLSServiceActivation(env *OLSTestEnvironment) (*corev1.Secret, error) {
	// Wait for the application service to be created
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerServiceName,
			Namespace: OLSNameSpace,
		},
	}
	err := env.Client.WaitForServiceCreated(service)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for service creation: %w", err)
	}

	// Check the secret holding TLS certificates is created
	secretName, ok := service.Annotations[ServiceAnnotationKeyTLSSecret]
	if !ok {
		return nil, fmt.Errorf("TLS secret annotation not found on service")
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: OLSNameSpace,
		},
	}
	err = env.Client.WaitForSecretCreated(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for secret creation: %w", err)
	}

	// Check the deployment has the certificate secret mounted
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AppServerDeploymentName,
			Namespace: OLSNameSpace,
		},
	}
	err = env.Client.Get(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	secretVolumeDefaultMode := int32(420)
	expectedVolume := corev1.Volume{
		Name: "secret-" + AppServerTLSSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &secretVolumeDefaultMode,
			},
		},
	}

	if !containsVolume(deployment.Spec.Template.Spec.Volumes, expectedVolume) {
		return nil, fmt.Errorf("expected volume not found in deployment")
	}

	return secret, nil
}

// TestHTTPSQueryEndpoint tests HTTPS POST on /v1/query endpoint
func TestHTTPSQueryEndpoint(env *OLSTestEnvironment, secret *corev1.Secret, requestBody []byte) (*http.Response, []byte, error) {
	certificate, ok := secret.Data["tls.crt"]
	if !ok {
		return nil, nil, fmt.Errorf("tls.crt not found in secret")
	}

	httpsClient := NewHTTPSClient(env.ForwardHost, InClusterHost, certificate, nil, nil)
	authHeader := map[string]string{"Authorization": "Bearer " + env.SAToken}

	resp, err := httpsClient.PostJson("/v1/query", requestBody, authHeader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make HTTPS request: %w", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return resp, body, nil
}

// CreateOLSRoute creates a route for the OLS application
func CreateOLSRoute(client *Client) error {
	consoleURL := os.Getenv("CONSOLE_URL")
	if !strings.Contains(consoleURL, ".apps") {
		return fmt.Errorf("invalid console URL format")
	}

	// Create OLS application host URL
	index := strings.Index(consoleURL, ".apps")
	hostURL := OLSRouteName + consoleURL[index:]

	_, err := client.createRoute(OLSRouteName, OLSNameSpace, hostURL)
	return err
}

// UpdateRapidastConfig updates the rapidast config file with host and token
func UpdateRapidastConfig(hostURL, token string) error {
	configContent, err := os.ReadFile("../../ols-rapidast-config.yaml")
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	newContent := strings.ReplaceAll(string(configContent), "$HOST", hostURL)
	newContent = strings.ReplaceAll(newContent, "$BEARER_TOKEN", token)

	err = os.WriteFile("../../ols-rapidast-config-updated.yaml", []byte(newContent), 0644)
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

// CleanupTLSTestEnvironment cleans up the test environment
func CleanupTLSTestEnvironment(env *OLSTestEnvironment, testName string) error {
	err := mustGather(testName)
	if err != nil {
		return fmt.Errorf("failed to gather test artifacts: %w", err)
	}

	for _, cleanUp := range env.CleanUpFuncs {
		cleanUp()
	}

	return nil
}

// CleanupOLSTestEnvironmentWithCRDeletion cleans up the test environment including CR deletion
func CleanupOLSTestEnvironmentWithCRDeletion(env *OLSTestEnvironment, testName string) error {
	err := CleanupTLSTestEnvironment(env, testName)
	if err != nil {
		return err
	}

	// Delete the OLSConfig CR and wait for complete deletion
	if env.CR != nil {
		if err := env.Client.DeleteAndWait(env.CR, 3*time.Minute); err != nil {
			return fmt.Errorf("failed to delete OLSConfig CR: %w", err)
		}
	}

	return nil
}

// containsVolume checks if a volume exists in the volumes slice
func containsVolume(volumes []corev1.Volume, target corev1.Volume) bool {
	for _, volume := range volumes {
		if volume.Name == target.Name &&
			volume.Secret != nil &&
			target.Secret != nil &&
			volume.Secret.SecretName == target.Secret.SecretName &&
			*volume.Secret.DefaultMode == *target.Secret.DefaultMode {
			return true
		}
	}
	return false
}

func Ptr[T any](v T) *T { return &v }

func WriteResourceToFile(client *Client, clusterDir string, filename string, resource string) error {
	ctx, cancel := context.WithCancel(client.ctx)
	defer cancel()
	// Create file and file handler
	f, err := os.OpenFile(clusterDir+"/"+filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filename, err)
	}
	defer func() { _ = f.Close() }()
	// Execute command and write output to file
	cmd, err := exec.CommandContext(ctx, "oc", "get", resource, "-n", OLSNameSpace, "--kubeconfig", client.kubeconfigPath, "-o", "yaml").Output()
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", filename, err)
	}
	_, _ = f.Write(cmd)
	return nil
}

func WriteLogsToFile(client *Client, clusterDir string) error {
	ctx, cancel := context.WithCancel(client.ctx)
	defer cancel()
	// Create file and file handler

	// Execute command and write output to file
	pod_names, err := exec.CommandContext(ctx, "oc", "get", "pods", "-o", "name", "--no-headers", "-n", OLSNameSpace, "--kubeconfig", client.kubeconfigPath).Output()
	if err != nil {
		fmt.Printf("failed to get pods: %s \n", err)
	}
	pods := strings.Split(string(pod_names), "\n")
	for _, SinglePod := range pods {
		if SinglePod != "" {
			f, err := os.OpenFile(clusterDir+"/"+SinglePod+".txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Errorf("failed to create %s: %w", SinglePod, err)
			}
			defer func() { _ = f.Close() }()
			cmd, err := exec.CommandContext(ctx, "oc", "logs", "-n", OLSNameSpace, SinglePod, "--kubeconfig", client.kubeconfigPath).Output()
			if err != nil {
				fmt.Printf("failed to get logs: %s \n", err)
			}
			_, _ = f.Write(cmd)
		}
		//}
	}

	return nil
}

func mustGather(test_case string) error {
	var client *Client
	client, err := GetClient(nil)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}
	// create artifact directory
	artifact_dir := os.Getenv(ArtifactDir)
	if artifact_dir == "" {
		artifact_dir = "/artifacts"
	}
	llmProvider := os.Getenv(LLMProviderEnvVar)
	clusterDir := artifact_dir + "/" + llmProvider + "/" + test_case
	err = os.MkdirAll(clusterDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create folder %w", err)
	}
	err = os.MkdirAll(clusterDir+"/pod", os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create folder %w", err)
	}

	filename := "pods.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "pods")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "services.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "services")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "deployments.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "deployments")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "replicasets.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "replicasets")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "routes.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "routes")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "rolebindings.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "rolebindings")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "serviceaccounts.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "serviceaccounts")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "olsconfig.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "olsconfig")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "clusterserviceversion.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "clusterserviceversion")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "installplan.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "installplan")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	filename = "configmap.yaml"
	err = WriteResourceToFile(client, clusterDir, filename, "configmap")
	if err != nil {
		fmt.Printf("failed to write to %s: %s \n", filename, err)
	}

	err = WriteLogsToFile(client, clusterDir)
	if err != nil {
		fmt.Printf("failed to write logs: %s \n", err)
	}
	return nil
}
