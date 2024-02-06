package controller

import (
	"crypto/sha256"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
)

func updateDeploymentAnnotations(deployment *appsv1.Deployment, annotations map[string]string) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		deployment.Annotations[k] = v
	}
}

func hashBytes(sourceStr []byte) (string, error) {
	hashFunc := sha256.New()
	_, err := hashFunc.Write(sourceStr)
	if err != nil {
		return "", fmt.Errorf("failed to generate hash %w", err)
	}
	return fmt.Sprintf("%x", hashFunc.Sum(nil)), nil
}
