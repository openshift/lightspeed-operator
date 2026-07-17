package otelcollector

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
		{Name: "reconcile OTEL Collector Postgres Secret", Task: reconcileOtelCollectorPostgresSecret},
		{Name: "reconcile OTEL Collector NetworkPolicy", Task: reconcileOtelCollectorNetworkPolicy},
	}, true)
}

// ReconcileOtelCollectorDeployment reconciles the OTEL Collector Service, TLS material,
// client connectivity ConfigMap, and Deployment (Phase 2).
func ReconcileOtelCollectorDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileOtelCollectorDeployment", []utils.ReconcileTask{
		{Name: "reconcile OTEL Collector Service", Task: reconcileOtelCollectorService},
		{Name: "reconcile OTEL Collector TLS Certs", Task: reconcileOtelCollectorTLSSecret},
		{Name: "reconcile OTEL Collector client ConfigMap", Task: reconcileOtelCollectorClientConfigMap},
		{Name: "reconcile OTEL Collector Deployment", Task: reconcileOtelCollectorDeployment},
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

func reconcileOtelCollectorClientConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateOtelCollectorClientConfigMap(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOtelCollectorClientConfigMap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorClientConfigMapName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating OTEL Collector client configmap", "configmap", cm.Name)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOtelCollectorClientConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOtelCollectorClientConfigMap, err)
	}

	if utils.ConfigMapEqual(foundCm, cm) &&
		reflect.DeepEqual(foundCm.Labels, cm.Labels) &&
		reflect.DeepEqual(foundCm.OwnerReferences, cm.OwnerReferences) {
		r.GetLogger().Info("OTEL Collector client configmap unchanged, reconciliation skipped", "configmap", cm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Labels = cm.Labels
	foundCm.OwnerReferences = cm.OwnerReferences
	if err := r.Update(ctx, foundCm); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOtelCollectorClientConfigMap, err)
	}
	r.GetLogger().Info("OTEL Collector client configmap reconciled", "configmap", cm.Name)
	return nil
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

// RestartOtelCollector refreshes the client connectivity ConfigMap (CA from the
// serving cert) and triggers a rolling restart of the collector deployment.
// The cert Secret watcher already calls this on TLS rotation.
func RestartOtelCollector(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	cr := &olsv1alpha1.OLSConfig{}
	if err := r.Get(ctx, client.ObjectKey{Name: utils.OLSConfigName}, cr); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOLSConfigForOtelCollectorClientConfigMapRefresh, err)
	}
	if err := reconcileOtelCollectorClientConfigMap(r, ctx, cr); err != nil {
		return err
	}

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
