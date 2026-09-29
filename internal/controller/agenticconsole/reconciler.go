// Package agenticconsole provides reconciliation logic for the OpenShift Lightspeed
// agentic console plugin.
package agenticconsole

import (
	"context"
	"errors"
	"fmt"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// ReconcileAgenticConsoleUIResources reconciles all resources except the deployment (Phase 1).
func ReconcileAgenticConsoleUIResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileAgenticConsoleUIResources", []utils.ReconcileTask{
		{Name: "reconcile Agentic Console Plugin ConfigMap", Task: reconcileAgenticConsoleUIConfigMap},
		{Name: "reconcile Agentic Console Plugin NetworkPolicy", Task: reconcileAgenticConsoleNetworkPolicy},
		{Name: "reconcile Agentic Console Plugin Service Account", Task: reconcileAgenticConsoleUIServiceAccount},
	}, true)
}

// ReconcileAgenticConsoleUIDeploymentAndPlugin reconciles the deployment and related resources (Phase 2).
func ReconcileAgenticConsoleUIDeploymentAndPlugin(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileAgenticConsoleUIDeploymentAndPlugin", []utils.ReconcileTask{
		{Name: "reconcile Agentic Console Plugin Deployment", Task: ReconcileAgenticConsoleUIDeployment},
		{Name: "reconcile Agentic Console Plugin Service", Task: reconcileAgenticConsoleUIService},
		{Name: "reconcile Agentic Console Plugin TLS Certs", Task: reconcileAgenticConsoleTLSSecret},
		{Name: "reconcile Agentic Console Plugin", Task: reconcileAgenticConsoleUIPlugin},
		{Name: "activate Agentic Console Plugin", Task: activateAgenticConsoleUI},
	}, false)
}

func reconcileAgenticConsoleUIConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateAgenticConsoleUIConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginConfigMap, err)
	}
	return utils.ReconcileConsolePluginConfigMap(r, ctx, cm)
}

func reconcileAgenticConsoleUIService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateAgenticConsoleUIService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginService, err)
	}
	return utils.ReconcileConsolePluginService(r, ctx, service)
}

// ReconcileAgenticConsoleUIDeployment reconciles the agentic console UI deployment.
func ReconcileAgenticConsoleUIDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	deployment, err := GenerateAgenticConsoleUIDeployment(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginDeployment, err)
	}
	return utils.ReconcileConsolePluginDeployment(r, ctx, deployment, RestartAgenticConsoleUI)
}

func reconcileAgenticConsoleUIPlugin(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	plugin, err := GenerateAgenticConsoleUIPlugin(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePlugin, err)
	}
	return utils.ReconcileConsolePluginCR(r, ctx, plugin)
}

func activateAgenticConsoleUI(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.ActivateConsolePlugin(r, ctx, utils.AgenticConsoleUIPluginName)
}

// RemoveAgenticConsole deactivates and deletes the agentic console plugin.
func RemoveAgenticConsole(r reconciler.Reconciler, ctx context.Context) error {
	// The ConsolePlugin is cluster-scoped; owned namespaced resources are not
	// removed by deleting it. Stop the Deployment first, then tear down the
	// Service and its serving certificate along with the remaining operands.
	var errs []error
	if err := utils.RemoveConsolePlugin(r, ctx, utils.AgenticConsoleUIPluginName); err != nil {
		errs = append(errs, err)
	}
	for _, obj := range []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUIDeploymentName, Namespace: r.GetNamespace()}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUIServiceName, Namespace: r.GetNamespace()}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUIServiceCertSecretName, Namespace: r.GetNamespace()}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUIConfigMapName, Namespace: r.GetNamespace()}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUIServiceAccountName, Namespace: r.GetNamespace()}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: utils.AgenticConsoleUINetworkPolicyName, Namespace: r.GetNamespace()}},
	} {
		if err := r.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("delete agentic console %T: %w", obj, err))
		}
	}
	return errors.Join(errs...)
}

func reconcileAgenticConsoleTLSSecret(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.WaitForConsolePluginTLSSecret(r, ctx, utils.AgenticConsoleUIServiceCertSecretName)
}

func reconcileAgenticConsoleNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateAgenticConsoleUINetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginNetworkPolicy, err)
	}
	return utils.ReconcileConsolePluginNetworkPolicy(r, ctx, np)
}

func reconcileAgenticConsoleUIServiceAccount(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := GenerateAgenticConsoleUIServiceAccount(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginServiceAccount, err)
	}
	return utils.ReconcileConsolePluginServiceAccount(r, ctx, sa)
}

// RestartAgenticConsoleUI triggers a rolling restart of the agentic console UI deployment.
func RestartAgenticConsoleUI(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	return utils.RestartConsolePluginDeployment(r, ctx, utils.AgenticConsoleUIDeploymentName, deployment...)
}

// ReconcileAgenticConsoleUI reconciles all agentic console UI resources (test helper).
func ReconcileAgenticConsoleUI(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileAgenticConsoleUI starts")

	if err := ReconcileAgenticConsoleUIResources(r, ctx, olsconfig); err != nil {
		return err
	}
	if err := ReconcileAgenticConsoleUIDeploymentAndPlugin(r, ctx, olsconfig); err != nil {
		return err
	}

	r.GetLogger().Info("reconcileAgenticConsoleUI completed")
	return nil
}
