package controller

import (
	"crypto/sha256"
	"fmt"
	"io"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func updateDeploymentAnnotations(deployment *appsv1.Deployment, annotations map[string]string) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		deployment.Annotations[k] = v
	}
}

func getSHAfromConfigmap(configmap *corev1.ConfigMap) string {
	values := []string{}
	for k, v := range configmap.Data {
		values = append(values, k+"="+v)
	}
	sort.Strings(values)
	return generateSHA(strings.Join(values, ";"))
}

func generateSHA(data string) string {
	hasher := sha256.New()
	_, err := io.WriteString(hasher, data)
	if err != nil {
		return ""
	}
	sha := hasher.Sum(nil)
	return fmt.Sprintf("%x", sha)
}
