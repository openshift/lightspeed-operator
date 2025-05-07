package e2e

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Ptr[T any](v T) *T { return &v }

func hashBytes(sourceStr []byte) (string, error) { // nolint:unused
	hashFunc := sha256.New()
	_, err := hashFunc.Write(sourceStr)
	if err != nil {
		return "", fmt.Errorf("failed to generate hash %w", err)
	}
	return fmt.Sprintf("%x", hashFunc.Sum(nil)), nil
}

func WriteResourceToFile(client *Client, clusterDir string, filename string, resource string) error {
	ctx, _ := context.WithCancel(client.ctx)
	// Create file and file handler
	f, err := os.OpenFile(clusterDir+"/"+filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", filename, err)
	}
	defer f.Close()
	// Execute command and write output to file
	cmd, err := exec.CommandContext(ctx, "oc", "get", resource, "-n", OLSNameSpace, "--kubeconfig", client.kubeconfigPath, "-o", "yaml").Output()
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", filename, err)
	}
	f.Write(cmd)
	return nil
}

func WriteLogsToFile(client *Client, clusterDir string) error {
	ctx, _ := context.WithCancel(client.ctx)
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
			defer f.Close()
			cmd, err := exec.CommandContext(ctx, "oc", "logs", "-n", OLSNameSpace, SinglePod, "--kubeconfig", client.kubeconfigPath).Output()
			if err != nil {
				fmt.Printf("failed to get logs: %s \n", err)
			}
			f.Write(cmd)
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
	clusterDir := "." + artifact_dir + "/" + llmProvider + "/" + test_case
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
