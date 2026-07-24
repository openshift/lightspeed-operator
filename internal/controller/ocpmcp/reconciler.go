package ocpmcp

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileResources reconciles Phase 1 standalone MCP resources.
// When introspectionEnabled is false, removes managed MCP resources instead.
func ReconcileResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	if !utils.BoolDeref(olsconfig.Spec.OLSConfig.IntrospectionEnabled, true) {
		r.GetLogger().Info("openshift-mcp-server disabled; removing operand resources")
		return Remove(r, ctx)
	}

	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileOpenShiftMCPServerResources", []utils.ReconcileTask{
		{Name: "reconcile openshift-mcp-server ConfigMap", Task: reconcileConfigMap},
		{Name: "reconcile openshift-mcp-server ServiceAccount", Task: reconcileServiceAccount},
		{Name: "reconcile openshift-mcp-server NetworkPolicy", Task: reconcileNetworkPolicy},
		{Name: "remove legacy openshift-mcp-server CA ConfigMap", Task: removeLegacyCAConfigMap},
	}, true)
}

// ReconcileDeployment reconciles Phase 2: Service, TLS material, and Deployment.
func ReconcileDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	if !utils.BoolDeref(olsconfig.Spec.OLSConfig.IntrospectionEnabled, true) {
		return nil
	}

	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileOpenShiftMCPServerDeployment", []utils.ReconcileTask{
		{Name: "reconcile openshift-mcp-server Service", Task: reconcileService},
		{Name: "reconcile openshift-mcp-server TLS Certs", Task: reconcileTLSSecret},
		{Name: "reconcile openshift-mcp-server Deployment", Task: reconcileDeployment},
	}, false)
}

// Remove deletes all operator-managed standalone MCP resources.
func Remove(r reconciler.Reconciler, ctx context.Context) error {
	return utils.RunDeleteTasks(r, ctx, "RemoveOpenShiftMCPServer", []utils.DeleteTask{
		{Name: "delete openshift-mcp-server deployment", Task: deleteDeployment},
		{Name: "delete openshift-mcp-server service", Task: deleteService},
		{Name: "delete openshift-mcp-server network policy", Task: deleteNetworkPolicy},
		{Name: "delete openshift-mcp-server configmap", Task: deleteConfigMap},
		{Name: "delete openshift-mcp-server service account", Task: deleteServiceAccount},
		{Name: "delete openshift-mcp-server TLS secret", Task: deleteTLSSecret},
		{Name: "delete legacy openshift-mcp-server CA ConfigMap", Task: deleteLegacyCAConfigMap},
	})
}

func reconcileConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateConfigMap(r, cr)
	if err != nil {
		return err
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerConfigCmName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating openshift-mcp-server configmap", "configmap", cm.Name)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOpenShiftMCPServerConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOpenShiftMCPServerConfigMap, err)
	}

	if utils.ConfigMapEqual(foundCm, cm) && reflect.DeepEqual(foundCm.Labels, cm.Labels) {
		r.GetLogger().Info("openshift-mcp-server configmap unchanged, reconciliation skipped", "configmap", cm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Labels = cm.Labels
	if err := r.Update(ctx, foundCm); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOpenShiftMCPServerConfigMap, err)
	}
	r.GetLogger().Info("openshift-mcp-server configmap reconciled", "configmap", cm.Name)
	return nil
}

func reconcileServiceAccount(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := GenerateServiceAccount(r, cr)
	if err != nil {
		return err
	}

	foundSA := &corev1.ServiceAccount{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerServiceAccountName, Namespace: r.GetNamespace()}, foundSA)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating openshift-mcp-server service account", "serviceAccount", sa.Name)
		if err := r.Create(ctx, sa); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOpenShiftMCPServerServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOpenShiftMCPServerServiceAccount, err)
	}

	r.GetLogger().Info("openshift-mcp-server service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func reconcileNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateNetworkPolicy(r, cr)
	if err != nil {
		return err
	}

	foundNP := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerNetworkPolicyName, Namespace: r.GetNamespace()}, foundNP)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating openshift-mcp-server network policy", "networkpolicy", np.Name)
		if err := r.Create(ctx, np); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOpenShiftMCPServerNetworkPolicy, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOpenShiftMCPServerNetworkPolicy, err)
	}

	if utils.NetworkPolicyEqual(np, foundNP) {
		r.GetLogger().Info("openshift-mcp-server network policy unchanged, reconciliation skipped", "networkpolicy", np.Name)
		return nil
	}

	foundNP.Labels = np.Labels
	foundNP.Spec = np.Spec
	if err := r.Update(ctx, foundNP); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOpenShiftMCPServerNetworkPolicy, err)
	}
	r.GetLogger().Info("openshift-mcp-server network policy reconciled", "networkpolicy", np.Name)
	return nil
}

func reconcileService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOpenShiftMCPServerService, err)
	}
	return utils.ReconcileConsolePluginService(r, ctx, service)
}

func reconcileTLSSecret(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.WaitForConsolePluginTLSSecret(r, ctx, utils.OpenShiftMCPServerCertsSecretName)
}

func reconcileDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GenerateDeployment(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateOpenShiftMCPServerDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating openshift-mcp-server deployment", "deployment", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateOpenShiftMCPServerDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetOpenShiftMCPServerDeployment, err)
	}

	if err := UpdateDeployment(r, ctx, existingDeployment, desiredDeployment); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOpenShiftMCPServerDeployment, err)
	}

	r.GetLogger().Info("openshift-mcp-server deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func deleteDeployment(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &appsv1.Deployment{}, utils.OpenShiftMCPServerDeploymentName)
}

func deleteService(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &corev1.Service{}, utils.OpenShiftMCPServerServiceName)
}

func deleteNetworkPolicy(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &networkingv1.NetworkPolicy{}, utils.OpenShiftMCPServerNetworkPolicyName)
}

func deleteConfigMap(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &corev1.ConfigMap{}, utils.OpenShiftMCPServerConfigCmName)
}

func deleteServiceAccount(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &corev1.ServiceAccount{}, utils.OpenShiftMCPServerServiceAccountName)
}

func deleteTLSSecret(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &corev1.Secret{}, utils.OpenShiftMCPServerCertsSecretName)
}

// removeLegacyCAConfigMap deletes the pre-handoff MCP inject-cabundle ConfigMap left on upgrade.
func removeLegacyCAConfigMap(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return deleteLegacyCAConfigMap(r, ctx)
}

func deleteLegacyCAConfigMap(r reconciler.Reconciler, ctx context.Context) error {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.LegacyOpenShiftMCPServerCAConfigMapName, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get legacy openshift-mcp-server CA ConfigMap: %w", err)
	}
	if err := r.Delete(ctx, cm); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete legacy openshift-mcp-server CA ConfigMap: %w", err)
	}
	r.GetLogger().Info("deleted legacy openshift-mcp-server CA ConfigMap", "configmap", cm.Name)
	return nil
}

func deleteNamespacedObject(r reconciler.Reconciler, ctx context.Context, obj client.Object, name string) error {
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: r.GetNamespace()}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}
