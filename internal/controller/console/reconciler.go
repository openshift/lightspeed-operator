// Package console provides reconciliation logic for the OpenShift Console UI plugin
// that integrates OpenShift Lightspeed into the OpenShift web console.
//
// This package manages:
//   - ConsolePlugin custom resource for UI integration
//   - Console UI deployment and pod lifecycle
//   - Service configuration for plugin serving
//   - ConfigMap for Nginx configuration
//   - TLS certificate management for secure connections
//   - Network policies for console security
//   - Integration with OpenShift Console operator
//
// The console plugin provides users with a chat interface directly in the OpenShift
// web console to interact with the Lightspeed AI assistant. The main entry points are
// ReconcileConsoleUI for setup and RemoveConsoleUI for cleanup.
package console

import (
	"context"
	"fmt"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	appsv1 "k8s.io/api/apps/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// ReconcileConsoleUIResources reconciles all resources except the deployment (Phase 1)
// Uses continue-on-error pattern since these resources are independent
func ReconcileConsoleUIResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileConsoleUIResources", []utils.ReconcileTask{
		{Name: "reconcile Console Plugin ConfigMap", Task: reconcileConsoleUIConfigMap},
		{Name: "reconcile Console Plugin NetworkPolicy", Task: reconcileConsoleNetworkPolicy},
		{Name: "reconcile Console Plugin Service Account", Task: reconcileConsoleUIServiceAccount},
	}, true)
}

// ReconcileConsoleUIDeploymentAndPlugin reconciles the deployment and related resources (Phase 2)
func ReconcileConsoleUIDeploymentAndPlugin(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileConsoleUIDeploymentAndPlugin", []utils.ReconcileTask{
		{Name: "reconcile Console Plugin Deployment", Task: ReconcileConsoleUIDeployment},
		{Name: "reconcile Console Plugin Service", Task: reconcileConsoleUIService},
		{Name: "reconcile Console Plugin TLS Certs", Task: reconcileConsoleTLSSecret},
		{Name: "reconcile Console Plugin", Task: reconcileConsoleUIPlugin},
		{Name: "activate Console Plugin", Task: activateConsoleUI},
	}, false)
}

func reconcileConsoleUIConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateConsoleUIConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginConfigMap, err)
	}
	return utils.ReconcileConsolePluginConfigMap(r, ctx, cm)
}

func reconcileConsoleUIService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateConsoleUIService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginService, err)
	}
	return utils.ReconcileConsolePluginService(r, ctx, service)
}

func ReconcileConsoleUIDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	deployment, err := GenerateConsoleUIDeployment(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginDeployment, err)
	}
	return utils.ReconcileConsolePluginDeployment(r, ctx, deployment, RestartConsoleUI)
}

func reconcileConsoleUIPlugin(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	plugin, err := GenerateConsoleUIPlugin(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePlugin, err)
	}
	return utils.ReconcileConsolePluginCR(r, ctx, plugin)
}

func activateConsoleUI(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.ActivateConsolePlugin(r, ctx, utils.ConsoleUIPluginName)
}

func RemoveConsoleUI(r reconciler.Reconciler, ctx context.Context) error {
	return utils.RemoveConsolePlugin(r, ctx, utils.ConsoleUIPluginName)
}

func reconcileConsoleTLSSecret(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.WaitForConsolePluginTLSSecret(r, ctx, utils.ConsoleUIServiceCertSecretName)
}

func reconcileConsoleNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateConsoleUINetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginNetworkPolicy, err)
	}
	return utils.ReconcileConsolePluginNetworkPolicy(r, ctx, np)
}

func reconcileConsoleUIServiceAccount(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := GenerateConsoleUIServiceAccount(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginServiceAccount, err)
	}
	return utils.ReconcileConsolePluginServiceAccount(r, ctx, sa)
}

// RestartConsoleUI triggers a rolling restart of the Console UI deployment by updating its pod template annotation.
// This is useful when configuration changes require a pod restart (e.g., ConfigMap or Secret updates).
func RestartConsoleUI(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	return utils.RestartConsolePluginDeployment(r, ctx, utils.ConsoleUIDeploymentName, deployment...)
}

// =============================================================================
// Test Helper Functions
// =============================================================================
// The following functions are convenience wrappers used primarily by unit tests.
// Production code should call ReconcileConsoleUIResources and ReconcileConsoleUIDeploymentAndPlugin directly.

// ReconcileConsoleUI reconciles all Console UI resources in the original order.
// This function is maintained for backward compatibility with existing tests.
// New code should call ReconcileConsoleUIResources and ReconcileConsoleUIDeploymentAndPlugin separately.
func ReconcileConsoleUI(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileConsoleUI starts")

	if err := ReconcileConsoleUIResources(r, ctx, olsconfig); err != nil {
		return err
	}
	if err := ReconcileConsoleUIDeploymentAndPlugin(r, ctx, olsconfig); err != nil {
		return err
	}

	r.GetLogger().Info("reconcileConsoleUI completed")
	return nil
}
