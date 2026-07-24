package agenticintegration

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileAgenticIntegrationResources reconciles the classic→agentic handoff ConfigMap.
// Client CA Secrets are owned by appserver.
//
// First create is gated on OTEL (and MCP when introspection is enabled) Service
// presence plus client CA Secrets. If the ConfigMap already exists, updates
// (sandbox-mode / PodSpec / keys) proceed even when those prerequisites are
// temporarily unmet. Watcher TouchAgenticConfiguration is ungated.
func ReconcileAgenticIntegrationResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileAgenticIntegrationResources", []utils.ReconcileTask{
		{Name: "reconcile agentic configuration ConfigMap", Task: reconcileConfigurationConfigMap},
	}, true)
}

func reconcileConfigurationConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateAgenticConfigurationConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAgenticConfigurationConfigMap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AgenticConfigurationConfigMapName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		if readyErr := handoffCreatePrerequisites(r, ctx, cr); readyErr != nil {
			r.GetLogger().Info("skipping agentic configuration create; prerequisites not ready",
				"configmap", cm.Name, "reason", readyErr.Error())
			return fmt.Errorf("%s: %w", utils.ErrAgenticConfigurationPrerequisitesNotReady, readyErr)
		}
		r.GetLogger().Info("creating agentic configuration configmap", "configmap", cm.Name)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAgenticConfigurationConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAgenticConfigurationConfigMap, err)
	}

	// Preserve cert-reload annotation set by TouchAgenticConfiguration.
	if foundCm.Annotations != nil {
		if v, ok := foundCm.Annotations[utils.AgenticConfigurationCertReloadAnnotation]; ok {
			if cm.Annotations == nil {
				cm.Annotations = map[string]string{}
			}
			cm.Annotations[utils.AgenticConfigurationCertReloadAnnotation] = v
		}
	}

	if utils.ConfigMapEqual(foundCm, cm) &&
		reflect.DeepEqual(foundCm.Labels, cm.Labels) &&
		reflect.DeepEqual(foundCm.Annotations, cm.Annotations) &&
		reflect.DeepEqual(foundCm.OwnerReferences, cm.OwnerReferences) {
		r.GetLogger().Info("agentic configuration configmap unchanged, reconciliation skipped", "configmap", cm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Labels = cm.Labels
	foundCm.Annotations = cm.Annotations
	foundCm.OwnerReferences = cm.OwnerReferences
	if err := r.Update(ctx, foundCm); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAgenticConfigurationConfigMap, err)
	}
	r.GetLogger().Info("agentic configuration configmap reconciled", "configmap", cm.Name)
	return nil
}

// handoffCreatePrerequisites checks cluster objects required before the first
// handoff ConfigMap publish. Services imply Phase 2 progressed far enough to
// advertise endpoints; client CA Secrets must exist so agentic can trust TLS.
func handoffCreatePrerequisites(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	ns := r.GetNamespace()

	svc := &corev1.Service{}
	if err := r.Get(ctx, client.ObjectKey{Name: utils.OtelCollectorServiceName, Namespace: ns}, svc); err != nil {
		return fmt.Errorf("OTEL Collector Service %s: %w", utils.OtelCollectorServiceName, err)
	}
	if err := requireSecretKey(r, ctx, ns, utils.AgenticOtelCASecretName, utils.AgenticOtelCASecretDataKey); err != nil {
		return err
	}

	if !utils.BoolDeref(cr.Spec.OLSConfig.IntrospectionEnabled, true) {
		return nil
	}

	if err := r.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerServiceName, Namespace: ns}, svc); err != nil {
		return fmt.Errorf("OpenShift MCP Service %s: %w", utils.OpenShiftMCPServerServiceName, err)
	}
	return requireSecretKey(r, ctx, ns, utils.AgenticMCPCASecretName, utils.AgenticMCPCASecretDataKey)
}

func requireSecretKey(r reconciler.Reconciler, ctx context.Context, namespace, name, key string) error {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret); err != nil {
		return fmt.Errorf("client CA Secret %s: %w", name, err)
	}
	if data, ok := secret.Data[key]; !ok || len(data) == 0 {
		return fmt.Errorf("client CA Secret %s key %q missing or empty", name, key)
	}
	return nil
}
