package utils

import (
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// RHOKPWaitMaxSeconds is the maximum time the wait-for-rhokp init container
// will poll before giving up.  Matches the RHOKP startup-probe budget
// (20 s initial-delay + 34 failures × 10 s period ≈ 360 s).
const RHOKPWaitMaxSeconds = 360

// GenerateRHOKPWaitInitContainer returns an init container that blocks until the
// standalone RHOKP/Solr instance is reachable on its HTTPS endpoint.
//
// Without this gate the app-server's SolrHybridSearch initializer may exhaust
// its 120 s retry budget before RHOKP is ready, permanently caching a nil
// reference for product-docs RAG (OLS-3799).
//
// The container uses the app-server image (which ships curl) and polls the
// Solr admin/ping endpoint.  The RHOKP CA certificate volume must already
// be defined on the pod (AppRHOKPCACertVolumeName).
func GenerateRHOKPWaitInitContainer(image, namespace string) corev1.Container {
	rhokpURL := RHOKPServiceURL(namespace) + RHOOKPReadinessHTTPPath

	caPath := path.Join(OLSAppCertsMountRoot, AppRHOKPCACertDir, AppRHOKPCACertFile)

	script := fmt.Sprintf(`
if ! command -v curl >/dev/null 2>&1; then
  echo "wait-for-rhokp: curl not found in image" >&2
  exit 1
fi

sleep_sec=1
max_sleep=30
start=$(date +%%s)
max_elapsed=%d
url="%s"
ca="%s"

backoff() {
  sleep "$sleep_sec"
  sleep_sec=$((sleep_sec * 2))
  [ "$sleep_sec" -gt "$max_sleep" ] && sleep_sec="$max_sleep"
}

while true; do
  now=$(date +%%s)
  elapsed=$((now - start))
  if [ "$elapsed" -ge "$max_elapsed" ]; then
    echo "wait-for-rhokp: timed out after ${max_elapsed}s" >&2
    exit 1
  fi

  if curl -sf --cacert "$ca" --max-time 5 "$url" >/dev/null 2>&1; then
    echo "wait-for-rhokp: RHOKP/Solr is reachable"
    exit 0
  fi

  echo "wait-for-rhokp: not ready yet (elapsed=${elapsed}s)" >&2
  backoff
done
`, RHOKPWaitMaxSeconds, rhokpURL, caPath)

	return corev1.Container{
		Name:            RHOKPWaitInitContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", script},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      AppRHOKPCACertVolumeName,
				MountPath: path.Join(OLSAppCertsMountRoot, AppRHOKPCACertDir),
				ReadOnly:  true,
			},
		},
		SecurityContext: RestrictedContainerSecurityContext(),
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
}
