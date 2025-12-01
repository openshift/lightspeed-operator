package watchers

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/appserver"
	"github.com/openshift/lightspeed-operator/internal/controller/console"
	"github.com/openshift/lightspeed-operator/internal/controller/lcore"
	"github.com/openshift/lightspeed-operator/internal/controller/postgres"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// isSecretReferencedInCR checks if a secret is referenced in the OLSConfig CR
func isSecretReferencedInCR(cr *olsv1alpha1.OLSConfig, secretName string) bool {
	found := false
	_ = utils.ForEachExternalSecret(cr, func(name, source string) error {
		if name == secretName {
			found = true
			return fmt.Errorf("stop") // Stop iteration
		}
		return nil
	})
	return found
}

// isConfigMapReferencedInCR checks if a configmap is referenced in the OLSConfig CR
func isConfigMapReferencedInCR(cr *olsv1alpha1.OLSConfig, cmName string) bool {
	found := false
	_ = utils.ForEachExternalConfigMap(cr, func(name, source string) error {
		if name == cmName {
			found = true
			return fmt.Errorf("stop") // Stop iteration
		}
		return nil
	})
	return found
}

// SecretUpdateHandler handles update events for Secrets and triggers deployment restarts when data changes.
type SecretUpdateHandler struct {
	Reconciler reconciler.Reconciler
}

// Create implements handler.EventHandler - handle creation of watched secrets
// This handles the case where a watched secret is created or recreated.
// Instead of triggering full reconciliation, we check if the secret is referenced in the CR,
// annotate it if needed, and directly trigger deployment restarts.
func (h *SecretUpdateHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.Object
	secret, ok := obj.(*v1.Secret)
	if !ok {
		return
	}

	// Skip operator-owned secrets - they're managed via Owns() relationship
	// Check if owned by OLSConfig CR
	for _, owner := range secret.GetOwnerReferences() {
		if owner.Kind == utils.OLSConfigKind && owner.APIVersion == utils.OLSConfigAPIVersion {
			return
		}
	}

	// Fetch the OLSConfig CR to check if this secret should be watched
	cr := &olsv1alpha1.OLSConfig{}
	err := h.Reconciler.Get(ctx, types.NamespacedName{Name: utils.OLSConfigName}, cr)
	if err != nil {
		// Can't get CR, skip
		return
	}

	// Check if this secret is referenced in the CR
	if !isSecretReferencedInCR(cr, secret.Name) {
		return
	}

	// Annotate the secret if not already annotated
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	if _, exists := secret.Annotations[utils.WatcherAnnotationKey]; !exists {
		utils.AnnotateSecretWatcher(secret)
		err = h.Reconciler.Update(ctx, secret)
		if err != nil {
			// Failed to annotate, skip restart
			return
		}
	}

	// Trigger deployment restarts for this recreated/created secret
	SecretWatcherFilter(h.Reconciler, ctx, obj)
}

// Update implements handler.EventHandler - this is where we check if secret data changed
func (h *SecretUpdateHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldSecret, oldOk := evt.ObjectOld.(*v1.Secret)
	newSecret, newOk := evt.ObjectNew.(*v1.Secret)

	if !oldOk || !newOk {
		return
	}

	// Check if the data actually changed (not just metadata/annotations)
	if apiequality.Semantic.DeepEqual(oldSecret.Data, newSecret.Data) {
		// Data hasn't changed, skip
		return
	}

	// Data changed - restart affected deployments directly
	SecretWatcherFilter(h.Reconciler, ctx, newSecret)
}

// Delete implements handler.EventHandler - we don't care about deletes
func (h *SecretUpdateHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// No-op: secret deletes are handled by reconciliation
}

// Generic implements handler.EventHandler - we don't use generic events
func (h *SecretUpdateHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// No-op
}

// ConfigMapUpdateHandler handles update events for ConfigMaps and triggers deployment restarts when data changes.
type ConfigMapUpdateHandler struct {
	Reconciler reconciler.Reconciler
}

// Create implements handler.EventHandler - handle creation of watched configmaps
// This handles the case where a watched configmap is created or recreated.
// Instead of triggering full reconciliation, we check if the configmap is referenced in the CR,
// annotate it if needed, and directly trigger deployment restarts.
func (h *ConfigMapUpdateHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	obj := evt.Object
	cm, ok := obj.(*v1.ConfigMap)
	if !ok {
		return
	}

	// Skip operator-owned configmaps - they're managed via Owns() relationship
	// Check if owned by OLSConfig CR
	for _, owner := range cm.GetOwnerReferences() {
		if owner.Kind == utils.OLSConfigKind && owner.APIVersion == utils.OLSConfigAPIVersion {
			return
		}
	}

	// Fetch the OLSConfig CR to check if this configmap should be watched
	cr := &olsv1alpha1.OLSConfig{}
	err := h.Reconciler.Get(ctx, types.NamespacedName{Name: utils.OLSConfigName}, cr)
	if err != nil {
		// Can't get CR, skip
		return
	}

	// Check if this configmap is referenced in the CR
	if !isConfigMapReferencedInCR(cr, cm.Name) {
		return
	}

	// Annotate the configmap if not already annotated
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}
	if _, exists := cm.Annotations[utils.WatcherAnnotationKey]; !exists {
		utils.AnnotateConfigMapWatcher(cm)
		err = h.Reconciler.Update(ctx, cm)
		if err != nil {
			// Failed to annotate, skip restart
			return
		}
	}

	// Trigger deployment restarts for this recreated/created configmap
	ConfigMapWatcherFilter(h.Reconciler, ctx, obj)
}

// Update implements handler.EventHandler - this is where we check if configmap data changed
func (h *ConfigMapUpdateHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	oldCM, oldOk := evt.ObjectOld.(*v1.ConfigMap)
	newCM, newOk := evt.ObjectNew.(*v1.ConfigMap)

	if !oldOk || !newOk {
		return
	}

	// Check if the data actually changed (not just metadata/annotations)
	if apiequality.Semantic.DeepEqual(oldCM.Data, newCM.Data) &&
		apiequality.Semantic.DeepEqual(oldCM.BinaryData, newCM.BinaryData) {
		// Data hasn't changed, skip
		return
	}

	// Data changed - restart affected deployments directly
	ConfigMapWatcherFilter(h.Reconciler, ctx, newCM)
}

// Delete implements handler.EventHandler - we don't care about deletes
func (h *ConfigMapUpdateHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// No-op: configmap deletes are handled by reconciliation
}

// Generic implements handler.EventHandler - we don't use generic events
func (h *ConfigMapUpdateHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// No-op
}

// SecretWatcherFilter is a data-driven filter function for watching Secrets.
// It uses the reconciler's WatcherConfig to determine which deployments to restart when a Secret changes.
//
// The filter handles two types of secrets:
// 1. System secrets (e.g., telemetry pull secret) - defined in WatcherConfig.Secrets.SystemResources
// 2. Annotated secrets (user-provided from CR) - marked with utils.WatcherAnnotationKey
//
// For system secrets, it uses the AffectedDeployments from WatcherConfig.
// For annotated secrets, it looks up the deployment mapping in WatcherConfig.AnnotatedSecretMapping.
//
// This function directly restarts affected deployments and does not trigger reconciliation.
func SecretWatcherFilter(r reconciler.Reconciler, ctx context.Context, obj client.Object, inCluster ...bool) {

	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	// Get watcherConfig and useLCore from reconciler
	watcherConfig, _ := r.GetWatcherConfig().(*utils.WatcherConfig)
	useLCore := r.UseLCore()

	// Check 1: Check against configured system secrets (no hardcoded values!)
	if watcherConfig != nil {
		for _, systemSecret := range watcherConfig.Secrets.SystemResources {
			if obj.GetNamespace() == systemSecret.Namespace && obj.GetName() == systemSecret.Name {
				r.GetLogger().Info("Detected system secret change",
					"secret", systemSecret.Name,
					"namespace", systemSecret.Namespace,
					"description", systemSecret.Description)

				// Restart all affected deployments
				if inClusterValue {
					restartDeployment(r, ctx, systemSecret.AffectedDeployments, systemSecret.Namespace, systemSecret.Name, useLCore)
				}
				return
			}
		}
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Check 2: Look for watcher annotation (user-provided secrets)
	if _, exist := annotations[utils.WatcherAnnotationKey]; exist {
		// For annotated secrets, determine affected deployments from mapping
		secretName := obj.GetName()
		var affectedDeployments []string
		var found bool
		if watcherConfig != nil {
			affectedDeployments, found = watcherConfig.AnnotatedSecretMapping[secretName]
		}
		if !found {
			// Default: affect only ACTIVE_BACKEND (e.g., LLM provider secrets)
			affectedDeployments = []string{"ACTIVE_BACKEND"}
		}

		r.GetLogger().Info("Detected annotated secret change",
			"secret", secretName,
			"affectedDeployments", affectedDeployments)
		if inClusterValue {
			restartDeployment(r, ctx, affectedDeployments, obj.GetNamespace(), secretName, useLCore)
		}
		return
	}
	// Not a watched secret - no reconciliation needed. Should never happen
}

// ConfigMapWatcherFilter is a data-driven filter function for watching ConfigMaps.
// It uses the reconciler's WatcherConfig to determine which deployments to restart when a ConfigMap changes.
//
// The filter handles two types of configmaps:
// 1. System configmaps (e.g., OpenShift CA bundle) - defined in WatcherConfig.ConfigMaps.SystemResources
// 2. Annotated configmaps (user-provided from CR) - marked with utils.WatcherAnnotationKey
//
// For system configmaps, it uses the AffectedDeployments from WatcherConfig.
// For annotated configmaps, it looks up the deployment mapping in WatcherConfig.AnnotatedConfigMapMapping.
//
// This function directly restarts affected deployments and does not trigger reconciliation.
func ConfigMapWatcherFilter(r reconciler.Reconciler, ctx context.Context, obj client.Object, inCluster ...bool) {

	// Set default value for inCluster
	inClusterValue := true
	if len(inCluster) > 0 {
		inClusterValue = inCluster[0]
	}

	// Get watcherConfig and useLCore from reconciler
	watcherConfig, _ := r.GetWatcherConfig().(*utils.WatcherConfig)
	useLCore := r.UseLCore()

	// Check 1: Check against configured system configmaps (no hardcoded values!)
	if watcherConfig != nil {
		for _, systemCM := range watcherConfig.ConfigMaps.SystemResources {
			if obj.GetNamespace() == systemCM.Namespace && obj.GetName() == systemCM.Name {
				r.GetLogger().Info("Detected system configmap change",
					"configmap", systemCM.Name,
					"namespace", systemCM.Namespace,
					"description", systemCM.Description)

				// Restart all affected deployments
				if inClusterValue {
					restartDeployment(r, ctx, systemCM.AffectedDeployments, systemCM.Namespace, systemCM.Name, useLCore)
				}
				return
			}
		}
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Check 2: Look for watcher annotation (user-provided configmaps)
	if _, exist := annotations[utils.WatcherAnnotationKey]; exist {
		// For annotated configmaps, determine affected deployments from mapping
		configMapName := obj.GetName()
		var affectedDeployments []string
		var found bool
		if watcherConfig != nil {
			affectedDeployments, found = watcherConfig.AnnotatedConfigMapMapping[configMapName]
		}
		if !found {
			// Default: affect only ACTIVE_BACKEND (e.g., CA bundle configmaps)
			affectedDeployments = []string{"ACTIVE_BACKEND"}
		}

		r.GetLogger().Info("Detected annotated configmap change",
			"configmap", configMapName,
			"affectedDeployments", affectedDeployments)
		if inClusterValue {
			restartDeployment(r, ctx, affectedDeployments, obj.GetNamespace(), configMapName, useLCore)
		}
		return
	}
	// Not a watched configmap - no reconciliation needed. Should never happen
}

// restart corresponding deployment
func restartDeployment(r reconciler.Reconciler, ctx context.Context, affectedDeployments []string, namespace string, name string, useLCore bool) {

	for _, depName := range affectedDeployments {
		// Resolve ACTIVE_BACKEND to actual deployment name
		if depName == "ACTIVE_BACKEND" {
			if useLCore {
				depName = utils.LCoreDeploymentName
			} else {
				depName = utils.OLSAppServerDeploymentName
			}
		}
		// Restart the deployment
		var err error
		switch depName {
		case utils.OLSAppServerDeploymentName:
			err = appserver.RestartAppServer(r, ctx)
		case utils.LCoreDeploymentName:
			err = lcore.RestartLCore(r, ctx)
		case utils.PostgresDeploymentName:
			err = postgres.RestartPostgres(r, ctx)
		case utils.ConsoleUIDeploymentName:
			err = console.RestartConsoleUI(r, ctx)
		default:
			r.GetLogger().Info("unknown deployment name", "deployment", depName)
			continue
		}

		if err != nil {
			r.GetLogger().Error(err, "failed to restart deployment",
				"deployment", depName, "resource", name, "namespace", namespace)
			// Continue with other deployments
		} else {
			r.GetLogger().Info("restarted deployment",
				"deployment", depName, "resource", name, "namespace", namespace)
		}
	}
}
