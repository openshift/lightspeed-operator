package otelcollector

import (
	"context"
	stderrors "errors"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileOtelCollectorResources reconciles Phase 1 OTEL Collector resources.
func ReconcileOtelCollectorResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileOtelCollectorResources", []utils.ReconcileTask{
		{Name: "reconcile OTEL Collector ConfigMap", Task: reconcileOtelCollectorConfigMap},
		{Name: "reconcile OTEL Collector ServiceAccount", Task: reconcileOtelCollectorServiceAccount},
		{Name: "reconcile OTEL Dataverse exporter resources", Task: reconcileOtelDataverseExporterResources},
		{Name: "reconcile OTEL Collector Postgres Secret", Task: reconcileOtelCollectorPostgresSecret},
		{Name: "reconcile OTEL Collector NetworkPolicy", Task: reconcileOtelCollectorNetworkPolicy},
		{Name: "remove legacy OTEL Collector client ConfigMap", Task: removeLegacyClientConfigMap},
	}, true)
}

// ReconcileOtelCollectorDeployment reconciles the OTEL Collector Service, TLS material,
// Deployment, and ServiceMonitor (Phase 2).
func ReconcileOtelCollectorDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileOtelCollectorDeployment", []utils.ReconcileTask{
		{Name: "reconcile OTEL Collector Service", Task: reconcileOtelCollectorService},
		{Name: "reconcile OTEL Collector TLS Certs", Task: reconcileOtelCollectorTLSSecret},
		{Name: "reconcile OTEL Collector Deployment", Task: reconcileOtelCollectorDeployment},
		{Name: "reconcile OTEL Collector ServiceMonitor", Task: reconcileOtelCollectorServiceMonitor},
	}, false)
}

func reconcileOtelCollectorConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateOtelCollectorConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorConfigMap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorConfigMapName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating OTEL Collector configmap", "configmap", cm.Name)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOtelCollectorConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorConfigMap, err)
	}

	if utils.ConfigMapEqual(foundCm, cm) {
		r.GetLogger().Info("OTEL Collector configmap unchanged, reconciliation skipped", "configmap", cm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Labels = cm.Labels
	if err := r.Update(ctx, foundCm); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOtelCollectorConfigMap, err)
	}
	r.GetLogger().Info("OTEL Collector configmap reconciled", "configmap", cm.Name)
	return nil
}

func reconcileOtelDataverseExporterResources(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	enabled, err := dataverseExporterEnabled(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("failed to determine OTEL Dataverse exporter enablement: %w", err)
	}
	if !enabled {
		return removeOtelDataverseExporterResources(r, ctx)
	}

	if err := reconcileOtelDataverseExporterConfigMap(r, ctx, cr); err != nil {
		return err
	}
	if err := reconcileOtelDataverseExporterClusterRole(r, ctx, cr); err != nil {
		return err
	}
	if err := reconcileOtelDataverseExporterClusterRoleBinding(r, ctx, cr); err != nil {
		return err
	}
	if err := reconcileOtelDataverseExporterPullSecretClusterRole(r, ctx, cr); err != nil {
		return err
	}
	return reconcileOtelDataverseExporterPullSecretRoleBinding(r, ctx, cr)
}

func reconcileOtelDataverseExporterConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desired, err := GenerateOtelDataverseExporterConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("failed to generate OTEL Dataverse exporter ConfigMap: %w", err)
	}

	existing := &corev1.ConfigMap{}
	key := client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}
	if err := r.Get(ctx, key, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create OTEL Dataverse exporter ConfigMap: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get OTEL Dataverse exporter ConfigMap: %w", err)
	}

	if reflect.DeepEqual(existing.Data, desired.Data) &&
		reflect.DeepEqual(existing.BinaryData, desired.BinaryData) &&
		reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
		return nil
	}

	existing.Data = desired.Data
	existing.BinaryData = desired.BinaryData
	existing.Labels = desired.Labels
	existing.OwnerReferences = desired.OwnerReferences
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update OTEL Dataverse exporter ConfigMap: %w", err)
	}
	return nil
}

func reconcileOtelDataverseExporterClusterRole(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desired, err := GenerateOtelDataverseExporterClusterRole(r, cr)
	if err != nil {
		return fmt.Errorf("failed to generate OTEL Dataverse exporter ClusterRole: %w", err)
	}

	existing := &rbacv1.ClusterRole{}
	if err := r.Get(ctx, client.ObjectKey{Name: desired.Name}, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create OTEL Dataverse exporter ClusterRole: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get OTEL Dataverse exporter ClusterRole: %w", err)
	}

	if reflect.DeepEqual(existing.Rules, desired.Rules) &&
		reflect.DeepEqual(existing.AggregationRule, desired.AggregationRule) &&
		reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
		return nil
	}

	existing.Rules = desired.Rules
	existing.AggregationRule = desired.AggregationRule
	existing.Labels = desired.Labels
	existing.OwnerReferences = desired.OwnerReferences
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update OTEL Dataverse exporter ClusterRole: %w", err)
	}
	return nil
}

func reconcileOtelDataverseExporterClusterRoleBinding(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desired, err := GenerateOtelDataverseExporterClusterRoleBinding(r, cr)
	if err != nil {
		return fmt.Errorf("failed to generate OTEL Dataverse exporter ClusterRoleBinding: %w", err)
	}

	existing := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, client.ObjectKey{Name: desired.Name}, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create OTEL Dataverse exporter ClusterRoleBinding: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get OTEL Dataverse exporter ClusterRoleBinding: %w", err)
	}

	if existing.RoleRef != desired.RoleRef {
		if err := r.Delete(ctx, existing); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to replace OTEL Dataverse exporter ClusterRoleBinding: %w", err)
		}
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to recreate OTEL Dataverse exporter ClusterRoleBinding: %w", err)
		}
		return nil
	}

	if reflect.DeepEqual(existing.Subjects, desired.Subjects) &&
		reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
		return nil
	}

	existing.Subjects = desired.Subjects
	existing.Labels = desired.Labels
	existing.OwnerReferences = desired.OwnerReferences
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update OTEL Dataverse exporter ClusterRoleBinding: %w", err)
	}
	return nil
}

func reconcileOtelDataverseExporterPullSecretClusterRole(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desired, err := GenerateOtelDataverseExporterPullSecretClusterRole(r, cr)
	if err != nil {
		return fmt.Errorf("failed to generate OTEL Dataverse exporter pull-secret ClusterRole: %w", err)
	}

	existing := &rbacv1.ClusterRole{}
	if err := r.Get(ctx, client.ObjectKey{Name: desired.Name}, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create OTEL Dataverse exporter pull-secret ClusterRole: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get OTEL Dataverse exporter pull-secret ClusterRole: %w", err)
	}

	if reflect.DeepEqual(existing.Rules, desired.Rules) &&
		reflect.DeepEqual(existing.AggregationRule, desired.AggregationRule) &&
		reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
		return nil
	}

	existing.Rules = desired.Rules
	existing.AggregationRule = desired.AggregationRule
	existing.Labels = desired.Labels
	existing.OwnerReferences = desired.OwnerReferences
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update OTEL Dataverse exporter pull-secret ClusterRole: %w", err)
	}
	return nil
}

func reconcileOtelDataverseExporterPullSecretRoleBinding(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desired, err := GenerateOtelDataverseExporterPullSecretRoleBinding(r, cr)
	if err != nil {
		return fmt.Errorf("failed to generate OTEL Dataverse exporter pull-secret RoleBinding: %w", err)
	}

	existing := &rbacv1.RoleBinding{}
	key := client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}
	if err := r.Get(ctx, key, existing); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return fmt.Errorf("failed to create OTEL Dataverse exporter pull-secret RoleBinding: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get OTEL Dataverse exporter pull-secret RoleBinding: %w", err)
	}

	if existing.RoleRef != desired.RoleRef {
		if err := r.Delete(ctx, existing); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("failed to replace OTEL Dataverse exporter pull-secret RoleBinding: %w", err)
		}
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to recreate OTEL Dataverse exporter pull-secret RoleBinding: %w", err)
		}
		return nil
	}

	if reflect.DeepEqual(existing.Subjects, desired.Subjects) &&
		reflect.DeepEqual(existing.Labels, desired.Labels) &&
		reflect.DeepEqual(existing.OwnerReferences, desired.OwnerReferences) {
		return nil
	}

	existing.Subjects = desired.Subjects
	existing.Labels = desired.Labels
	existing.OwnerReferences = desired.OwnerReferences
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update OTEL Dataverse exporter pull-secret RoleBinding: %w", err)
	}
	return nil
}

func removeOtelDataverseExporterResources(r reconciler.Reconciler, ctx context.Context) error {
	var errs []error
	objects := []client.Object{
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelDataverseExporterPullSecretRoleBindingName,
			Namespace: utils.TelemetryPullSecretNamespace,
		}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: utils.OtelDataverseExporterClusterRoleBindingName}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: utils.OtelDataverseExporterPullSecretClusterRoleName}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: utils.OtelDataverseExporterClusterRoleName}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelDataverseExporterConfigMapName,
			Namespace: r.GetNamespace(),
		}},
	}
	for _, obj := range objects {
		if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to delete OTEL Dataverse exporter resource %T %s: %w", obj, obj.GetName(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to remove OTEL Dataverse exporter resources: %w", stderrors.Join(errs...))
	}
	return nil
}

func reconcileOtelCollectorServiceAccount(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := GenerateOtelCollectorServiceAccount(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorServiceAccount, err)
	}

	foundSA := &corev1.ServiceAccount{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorServiceAccountName, Namespace: r.GetNamespace()}, foundSA)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating OTEL Collector service account", "serviceAccount", sa.Name)
		if err := r.Create(ctx, sa); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOtelCollectorServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorServiceAccount, err)
	}

	r.GetLogger().Info("OTEL Collector service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func reconcileOtelCollectorPostgresSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := GenerateOtelCollectorPostgresSecret(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorPostgresSecret, err)
	}

	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorPostgresDSNSecretName, Namespace: r.GetNamespace()}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating OTEL Collector Postgres secret", "secret", secret.Name)
		if err := r.Create(ctx, secret); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOtelCollectorPostgresSecret, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorPostgresSecret, err)
	}

	if reflect.DeepEqual(foundSecret.Data, secret.Data) && reflect.DeepEqual(foundSecret.Labels, secret.Labels) {
		r.GetLogger().Info("OTEL Collector Postgres secret unchanged, reconciliation skipped", "secret", secret.Name)
		return nil
	}

	foundSecret.Data = secret.Data
	foundSecret.Labels = secret.Labels
	if err := r.Update(ctx, foundSecret); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOtelCollectorPostgresSecret, err)
	}
	r.GetLogger().Info("OTEL Collector Postgres secret reconciled", "secret", secret.Name)
	return nil
}

func reconcileOtelCollectorNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateOtelCollectorNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorNetworkPolicy, err)
	}

	foundNP := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorNetworkPolicyName, Namespace: r.GetNamespace()}, foundNP)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating OTEL Collector network policy", "networkpolicy", np.Name)
		if err := r.Create(ctx, np); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOtelCollectorNetworkPolicy, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorNetworkPolicy, err)
	}

	if utils.NetworkPolicyEqual(np, foundNP) {
		r.GetLogger().Info("OTEL Collector network policy unchanged, reconciliation skipped", "networkpolicy", np.Name)
		return nil
	}

	foundNP.Labels = np.Labels
	foundNP.Spec = np.Spec
	if err := r.Update(ctx, foundNP); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOtelCollectorNetworkPolicy, err)
	}
	r.GetLogger().Info("OTEL Collector network policy reconciled", "networkpolicy", np.Name)
	return nil
}

func reconcileOtelCollectorService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateOtelCollectorService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorService, err)
	}

	return utils.ReconcileConsolePluginService(r, ctx, service)
}

func reconcileOtelCollectorTLSSecret(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.WaitForConsolePluginTLSSecret(r, ctx, utils.OtelCollectorCertsSecretName)
}

func reconcileOtelCollectorDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GenerateOtelCollectorDeployment(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating OTEL Collector deployment", "deployment", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOtelCollectorDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorDeployment, err)
	}

	if err := UpdateOtelCollectorDeployment(r, ctx, existingDeployment, desiredDeployment); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOtelCollectorDeployment, err)
	}

	r.GetLogger().Info("OTEL Collector deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func reconcileOtelCollectorServiceMonitor(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sm, err := generateOtelCollectorServiceMonitor(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorServiceMonitor, err)
	}
	return utils.ReconcileServiceMonitor(r, ctx, sm)
}

// removeLegacyClientConfigMap deletes the pre-handoff OTEL client ConfigMap left on upgrade.
func removeLegacyClientConfigMap(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.LegacyOtelCollectorClientConfigMapName, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get legacy OTEL Collector client ConfigMap: %w", err)
	}
	if err := r.Delete(ctx, cm); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete legacy OTEL Collector client ConfigMap: %w", err)
	}
	r.GetLogger().Info("deleted legacy OTEL Collector client ConfigMap", "configmap", cm.Name)
	return nil
}

// RestartOtelCollector triggers a rolling restart of the collector deployment.
// The cert Secret watcher already calls this on TLS rotation; app-server restart
// (same watcher) refreshes client CA Secrets and touches the agentic ConfigMap.
func RestartOtelCollector(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorDeploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorDeployment, err)
		}
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}

	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("triggering OTEL Collector rolling restart", "deployment", dep.Name)
	if err := r.Update(ctx, dep); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOtelCollectorDeployment, err)
	}

	return nil
}
