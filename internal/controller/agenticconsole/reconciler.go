// Package agenticconsole provides reconciliation logic for the OpenShift Lightspeed
// agentic console plugin.
package agenticconsole

import (
	"context"
	"fmt"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	appsv1 "k8s.io/api/apps/v1"

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
	return utils.RemoveConsolePlugin(r, ctx, utils.AgenticConsoleUIPluginName)
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
