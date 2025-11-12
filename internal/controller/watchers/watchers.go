package watchers

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/lightspeed-operator/internal/controller/appserver"
	"github.com/openshift/lightspeed-operator/internal/controller/lcore"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// SecretWatcherFilter is a filter function for watching Secrets with the watcher annotation
// or specific system Secrets (e.g., telemetry pull secret).
// It returns reconcile requests for OLSConfig resources that should be reconciled when the Secret changes.
func SecretWatcherFilter(ctx context.Context, obj client.Object) []reconcile.Request {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	skip := true
	// Check for annotation
	crName, exist := annotations[utils.WatcherAnnotationKey]
	if exist {
		skip = false
	}
	// Check for telemetry pull secret by namespace and name
	if obj.GetNamespace() == utils.TelemetryPullSecretNamespace && obj.GetName() == utils.TelemetryPullSecretName {
		crName = utils.OLSConfigName
		skip = false
	}

	if skip {
		return nil
	}

	// Trigger reconciliation request
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: crName,
		}},
	}
}

// ConfigMapWatcherFilter is a filter function for watching ConfigMaps with the watcher annotation
// or specific system ConfigMaps (e.g., CA bundles).
// When a watched ConfigMap changes, it triggers a rolling restart of the appropriate backend deployment
// and returns a reconcile request for the OLSConfig.
// Parameters:
//   - useLCore: if true, restarts LCore deployment; if false, restarts AppServer deployment
//   - inCluster: if false, skips deployment restart (for local development)
func ConfigMapWatcherFilter(r reconciler.Reconciler, ctx context.Context, obj client.Object, useLCore bool, inCluster ...bool) []reconcile.Request {
	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	skip := true
	// Check for annotation
	crName, exist := annotations[utils.WatcherAnnotationKey]
	if exist {
		skip = false
	}
	// Check for name as well. We need a configmap containing a CA bundle that can be used to verify the kube-apiserver
	if obj.GetName() == utils.DefaultOpenShiftCerts {
		crName = utils.OLSConfigName
		skip = false
	}

	if skip {
		return nil
	}

	// Restart the appropriate backend if running in cluster (not during local development)
	if inClusterValue {
		var err error
		if useLCore {
			// Restart LCore deployment
			err = lcore.RestartLCore(r, ctx)
			if err != nil {
				r.GetLogger().Info("failed to restart LCore", "error", err)
				// Don't return nil here - we still want to trigger reconciliation
			}
		} else {
			// Restart app server deployment
			err = appserver.RestartAppServer(r, ctx)
			if err != nil {
				r.GetLogger().Info("failed to restart app server", "error", err)
				// Don't return nil here - we still want to trigger reconciliation
			}
		}
	}

	// Trigger reconciliation request
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: crName,
		}},
	}
}

// PostgresCAWatcherFilter is a filter function for watching PostgreSQL CA certificate resources.
// It watches for changes to ConfigMap with the PostgreSQL CA certificate bundle and
// Secret with the PostgreSQL serving certificate. It returns reconcile requests for the
// OLSConfig resource when these resources change.
func PostgresCAWatcherFilter(r reconciler.Reconciler, ctx context.Context, obj client.Object) []reconcile.Request {
	// Only watch resources in the operator's namespace
	if obj.GetNamespace() != r.GetNamespace() {
		return nil
	}

	name := obj.GetName()
	if name != utils.OLSCAConfigMap && name != utils.PostgresCertsSecretName {
		return nil
	}
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: utils.OLSConfigName,
		}},
	}
}
