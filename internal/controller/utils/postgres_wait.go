package utils

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PostgresWaitMaxSeconds is the maximum time the wait init container will run before exiting with an error.
const PostgresWaitMaxSeconds = 300

// GeneratePostgresWaitInitContainer returns an init container that waits until the Postgres
// database is accepting connections before the main container starts.
//
// Uses pg_isready to probe the database directly, which is more reliable than checking
// Kubernetes deployment replica counts. The container uses the postgres image since
// pg_isready is bundled with it.
//
// Connection parameters are configured via environment variables (PGHOST, PGPORT, PGUSER, PGDATABASE)
// with sensible defaults. Retries with exponential backoff up to PostgresWaitMaxSeconds.
func GeneratePostgresWaitInitContainer(image string) corev1.Container {
	script := fmt.Sprintf(`
: "${PGHOST:=%s}"
: "${PGPORT:=%d}"
: "${PGUSER:=postgres}"
: "${PGDATABASE:=postgres}"

sleep_sec=1
max_sleep=30
start=$(date +%%s)
max_elapsed=%d

backoff() {
  sleep "$sleep_sec"
  sleep_sec=$((sleep_sec * 2))
  [ "$sleep_sec" -gt "$max_sleep" ] && sleep_sec="$max_sleep"
}

if ! command -v pg_isready >/dev/null 2>&1; then
  echo "wait-for-postgres: pg_isready not found in image" >&2
  exit 1
fi

while true; do
  now=$(date +%%s)
  elapsed=$((now - start))
  if [ "$elapsed" -ge "$max_elapsed" ]; then
    echo "wait-for-postgres: timed out after ${max_elapsed}s" >&2
    exit 1
  fi

  if pg_isready -q -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE"; then
    echo "wait-for-postgres: postgres is accepting connections"
    exit 0
  fi

  echo "wait-for-postgres: not ready yet (host=$PGHOST port=$PGPORT db=$PGDATABASE user=$PGUSER)" >&2
  backoff
done
`, PostgresServiceName, PostgresServicePort, PostgresWaitMaxSeconds)

	return corev1.Container{
		Name:            PostgresWaitInitContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c", script},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &[]bool{false}[0],
			ReadOnlyRootFilesystem:   &[]bool{true}[0],
		},
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
