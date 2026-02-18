package appserver

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func generateRAGVolume() corev1.Volume {
	return corev1.Volume{
		Name: utils.RAGVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func GenerateRAGInitContainers(cr *olsv1alpha1.OLSConfig) []corev1.Container {
	var initContainers []corev1.Container
	for idx, rag := range cr.Spec.OLSConfig.RAG {
		ragName := fmt.Sprintf("rag-%d", idx)
		initContainers = append(initContainers, corev1.Container{
			Name:            ragName,
			Image:           rag.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         []string{"sh", "-c", fmt.Sprintf("mkdir -p %s && cp -a %s/. %s", path.Join(utils.RAGVolumeMountPath, ragName), rag.IndexPath, path.Join(utils.RAGVolumeMountPath, ragName))},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      utils.RAGVolumeName,
					MountPath: utils.RAGVolumeMountPath,
				},
			},
		})
	}
	return initContainers
}

func generateRAGVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      utils.RAGVolumeName,
		MountPath: utils.RAGVolumeMountPath,
	}
}

// generatePostgresSafetyCheckInitContainer creates an init container that ensures
// Postgres is in a safe state before the app server starts. This prevents cache
// consistency issues when Postgres is restarting or scaling.
//
// The init container checks:
//  1. If Postgres deployment exists (may be disabled/deleted - safe to start)
//  2. If Postgres has exactly 1 ready replica (stable single instance - safe)
//  3. If Postgres is in a transitional state (restarting/scaling - wait)
//
// This approach moves the responsibility of startup safety from the operator
// to the app server itself, following Kubernetes best practices.
func generatePostgresSafetyCheckInitContainer(r reconciler.Reconciler) corev1.Container {
	return corev1.Container{
		Name:    "postgres-safety-check",
		Image:   "registry.redhat.io/openshift4/ose-cli:latest",
		Command: []string{"/bin/bash", "-c"},
		Args: []string{
			`#!/bin/bash
set -e

DEPLOYMENT="` + utils.PostgresDeploymentName + `"
NAMESPACE="${POD_NAMESPACE}"
MAX_RETRIES=60
RETRY_INTERVAL=5

echo "Checking Postgres deployment safety before starting app server..."

for i in $(seq 1 $MAX_RETRIES); do
    # Check if deployment exists
    if ! kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" &>/dev/null; then
        echo "✓ Postgres deployment not found - assuming disabled, safe to start"
        exit 0
    fi

    # Get deployment status
    DESIRED=$(kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "0")
    READY=$(kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
    UPDATED=$(kubectl get deployment "$DEPLOYMENT" -n "$NAMESPACE" -o jsonpath='{.status.updatedReplicas}' 2>/dev/null || echo "0")

    # Default to 0 if not set
    DESIRED=${DESIRED:-0}
    READY=${READY:-0}
    UPDATED=${UPDATED:-0}

    # Check safe states
    if [ "$DESIRED" -eq 0 ] && [ "$READY" -eq 0 ]; then
        echo "✓ Postgres disabled (0 replicas), safe to start"
        exit 0
    fi

    if [ "$DESIRED" -eq 1 ] && [ "$READY" -eq 1 ] && [ "$UPDATED" -eq 1 ]; then
        echo "✓ Postgres stable (1 ready replica), safe to start"
        exit 0
    fi

    # Unsafe state - Postgres is restarting or in transition
    echo "⏳ Postgres not stable (desired: $DESIRED, ready: $READY, updated: $UPDATED)"
    echo "   Retry $i/$MAX_RETRIES in ${RETRY_INTERVAL}s..."
    sleep $RETRY_INTERVAL
done

echo "❌ ERROR: Postgres did not reach safe state after $((MAX_RETRIES * RETRY_INTERVAL))s"
echo "   This usually means Postgres is stuck in a restart loop or has configuration issues."
exit 1
`,
		},
		Env: []corev1.EnvVar{
			{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("10m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
	}
}
