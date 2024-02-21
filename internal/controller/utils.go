package controller

import (
	"crypto/sha256"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

// updateDeploymentAnnotations updates the annotations in a given deployment.
func updateDeploymentAnnotations(deployment *appsv1.Deployment, annotations map[string]string) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		deployment.Annotations[k] = v
	}
}

func updateDeploymentTemplateAnnotations(deployment *appsv1.Deployment, annotations map[string]string) {
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		deployment.Spec.Template.Annotations[k] = v
	}
}

// setDeploymentReplicas sets the number of replicas in a given deployment.
func setDeploymentReplicas(deployment *appsv1.Deployment, replicas int32) bool {
	if *deployment.Spec.Replicas != replicas {
		*deployment.Spec.Replicas = replicas
		return true
	}

	return false
}

// setDeploymentContainerResources sets the resource requirements for a specific container in a given deployment.
func setDeploymentContainerResources(deployment *appsv1.Deployment, resources *corev1.ResourceRequirements, containerName string) (bool, error) {
	containerIndex, err := getContainerIndex(deployment, containerName)
	if err != nil {
		return false, err
	}
	existingResources := &deployment.Spec.Template.Spec.Containers[containerIndex].Resources
	desiredResources := *resources
	if !apiequality.Semantic.DeepEqual(*existingResources, desiredResources) {
		*existingResources = desiredResources
		return true, nil
	}

	return false, nil
}

// getContainerIndex returns the index of the container with the specified name in a given deployment.
func getContainerIndex(deployment *appsv1.Deployment, containerName string) (int, error) {
	for i, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			return i, nil
		}
	}
	return -1, fmt.Errorf("container %s not found in deployment %s", containerName, deployment.Name)
}

func hashBytes(sourceStr []byte) (string, error) {
	hashFunc := sha256.New()
	_, err := hashFunc.Write(sourceStr)
	if err != nil {
		return "", fmt.Errorf("failed to generate hash %w", err)
	}
	return fmt.Sprintf("%x", hashFunc.Sum(nil)), nil
}
