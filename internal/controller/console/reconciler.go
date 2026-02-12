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
	"slices"
	"time"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// ReconcileConsoleUIResources reconciles all resources except the deployment (Phase 1)
// Uses continue-on-error pattern since these resources are independent
func ReconcileConsoleUIResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileConsoleUIResources starts")
	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile Console Plugin ConfigMap",
			Task: reconcileConsoleUIConfigMap,
		},
		{
			Name: "reconcile Console Plugin NetworkPolicy",
			Task: reconcileConsoleNetworkPolicy,
		},
	}

	failedTasks := make(map[string]error)

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileConsoleUIResources error", "task", task.Name)
			failedTasks[task.Name] = err
		}
	}

	if len(failedTasks) > 0 {
		taskNames := make([]string, 0, len(failedTasks))
		for taskName, err := range failedTasks {
			taskNames = append(taskNames, taskName)
			r.GetLogger().Error(err, "Task failed in reconcileConsoleUIResources", "task", taskName)
		}
		return fmt.Errorf("failed tasks: %v", taskNames)
	}

	r.GetLogger().Info("reconcileConsoleUIResources completes")
	return nil
}

// ReconcileConsoleUIDeploymentAndPlugin reconciles the deployment and related resources (Phase 2)
func ReconcileConsoleUIDeploymentAndPlugin(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileConsoleUIDeploymentAndPlugin starts")

	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile Console Plugin Deployment",
			Task: ReconcileConsoleUIDeployment,
		},
		{
			Name: "reconcile Console Plugin Service",
			Task: reconcileConsoleUIService,
		},
		{
			Name: "reconcile Console Plugin TLS Certs",
			Task: reconcileConsoleTLSSecret,
		},
		{
			Name: "reconcile Console Plugin",
			Task: reconcileConsoleUIPlugin,
		},
		{
			Name: "activate Console Plugin",
			Task: activateConsoleUI,
		},
	}

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileConsoleUIDeploymentAndPlugin error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("reconcileConsoleUIDeploymentAndPlugin completes")
	return nil
}

func reconcileConsoleUIConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateConsoleUIConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginConfigMap, err)
	}
	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUIConfigMapName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console UI configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateConsolePluginConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetConsolePluginConfigMap, err)
	}

	if apiequality.Semantic.DeepEqual(foundCm.Data, cm.Data) {
		r.GetLogger().Info("Console UI configmap unchanged, reconciliation skipped", "configmap", cm.Name)
		return nil
	}
	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsolePluginConfigMap, err)
	}
	r.GetLogger().Info("Console configmap reconciled", "configmap", cm.Name)

	return nil
}

func reconcileConsoleUIService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateConsoleUIService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginService, err)
	}
	foundService := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUIServiceName, Namespace: r.GetNamespace()}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console UI service", "service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateConsolePluginService, err)
		}
		r.GetLogger().Info("Console UI service created", "service", service.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetConsolePluginService, err)
	}

	if utils.ServiceEqual(foundService, service) &&
		foundService.Annotations != nil &&
		foundService.Annotations[utils.ServingCertSecretAnnotationKey] == service.Annotations[utils.ServingCertSecretAnnotationKey] {
		r.GetLogger().Info("Console UI service unchanged, reconciliation skipped", "service", service.Name)
		return nil
	}

	err = r.Update(ctx, service)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsolePluginService, err)
	}

	r.GetLogger().Info("Console UI service reconciled", "service", service.Name)

	return nil
}

func ReconcileConsoleUIDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	deployment, err := GenerateConsoleUIDeployment(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginDeployment, err)
	}
	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUIDeploymentName, Namespace: r.GetNamespace()}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console UI deployment", "deployment", deployment.Name)
		err = r.Create(ctx, deployment)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateConsolePluginDeployment, err)
		}
		r.GetLogger().Info("Console UI deployment created", "deployment", deployment.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetConsolePluginDeployment, err)
	}

	// fill in the default values for the deployment for comparison
	utils.SetDefaults_Deployment(deployment)

	// Check if deployment spec has changed
	// Note: TLS secret changes are handled by watchers, not here
	if utils.DeploymentSpecEqual(&foundDeployment.Spec, &deployment.Spec, true) {
		return nil
	}

	// Apply the desired spec to the existing deployment
	foundDeployment.Spec = deployment.Spec

	r.GetLogger().Info("Updating Console UI deployment", "deployment", foundDeployment.Name)
	err = RestartConsoleUI(r, ctx, foundDeployment)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsolePluginDeployment, err)
	}
	r.GetLogger().Info("Console UI deployment reconciled", "deployment", deployment.Name)

	return nil
}

func reconcileConsoleUIPlugin(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	plugin, err := GenerateConsoleUIPlugin(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePlugin, err)
	}
	foundPlugin := &consolev1.ConsolePlugin{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUIPluginName}, foundPlugin)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console Plugin", "plugin", plugin.Name)
		err = r.Create(ctx, plugin)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateConsolePlugin, err)
		}
		r.GetLogger().Info("Console Plugin created", "plugin", plugin.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetConsolePlugin, err)
	}

	if apiequality.Semantic.DeepEqual(foundPlugin.Spec, plugin.Spec) {
		r.GetLogger().Info("Console Plugin unchanged, reconciliation skipped", "plugin", plugin.Name)
		return nil
	}

	foundPlugin.Spec = plugin.Spec
	err = r.Update(ctx, foundPlugin)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsolePlugin, err)
	}
	r.GetLogger().Info("Console Plugin reconciled", "plugin", plugin.Name)

	return nil
}

func activateConsoleUI(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		console := &openshiftv1.Console{}
		err := r.Get(ctx, client.ObjectKey{Name: utils.ConsoleCRName}, console)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrGetConsole, err)
		}
		if console.Spec.Plugins == nil {
			console.Spec.Plugins = []string{utils.ConsoleUIPluginName}
		} else if !slices.Contains(console.Spec.Plugins, utils.ConsoleUIPluginName) {
			console.Spec.Plugins = append(console.Spec.Plugins, utils.ConsoleUIPluginName)
		} else {
			return nil
		}

		return r.Update(ctx, console)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsole, err)
	}
	r.GetLogger().Info("Console UI plugin activated")
	return nil
}

func RemoveConsoleUI(r reconciler.Reconciler, ctx context.Context) error {
	tasks := []utils.DeleteTask{
		{
			Name: "deactivate Console Plugin",
			Task: deactivateConsoleUI,
		},
		{
			Name: "delete Console Plugin",
			Task: deleteConsoleUIPlugin,
		},
	}

	for _, task := range tasks {
		err := task.Task(r, ctx)
		if err != nil {
			r.GetLogger().Error(err, "DeleteConsoleUIPlugin error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("DeleteConsoleUIPlugin completed")

	return nil
}

func deleteConsoleUIPlugin(r reconciler.Reconciler, ctx context.Context) error {
	plugin := &consolev1.ConsolePlugin{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUIPluginName}, plugin)
	if err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("Console Plugin not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetConsolePlugin, err)
	}
	err = r.Delete(ctx, plugin)
	if err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("Console Plugin not found, consider deletion successful")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrDeleteConsolePlugin, err)
	}
	r.GetLogger().Info("Console Plugin deleted")
	return nil
}

func deactivateConsoleUI(r reconciler.Reconciler, ctx context.Context) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		console := &openshiftv1.Console{}
		err := r.Get(ctx, client.ObjectKey{Name: utils.ConsoleCRName}, console)
		if err != nil {
			// If Console CR doesn't exist, there's nothing to deactivate
			// This can happen in non-OpenShift environments or test scenarios
			if errors.IsNotFound(err) {
				r.GetLogger().Info("Console CR not found, skipping plugin deactivation")
				return nil
			}
			return fmt.Errorf("%s: %w", utils.ErrGetConsole, err)
		}
		if console.Spec.Plugins == nil {
			return nil
		}
		if slices.Contains(console.Spec.Plugins, utils.ConsoleUIPluginName) {
			console.Spec.Plugins = slices.DeleteFunc(console.Spec.Plugins, func(name string) bool { return name == utils.ConsoleUIPluginName })
		} else {
			return nil
		}
		return r.Update(ctx, console)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsole, err)
	}
	r.GetLogger().Info("Console UI plugin deactivated")
	return nil
}

func reconcileConsoleTLSSecret(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	foundSecret := &corev1.Secret{}
	var err, lastErr error
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, utils.ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		_, err = utils.GetSecretContent(r, utils.ConsoleUIServiceCertSecretName, r.GetNamespace(), []string{"tls.key", "tls.crt"}, foundSecret)
		if err != nil {
			lastErr = fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", utils.ConsoleUIServiceCertSecretName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to get TLS key and cert - wait err %w; last error: %w", err, lastErr)
	}
	err = r.Update(ctx, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	r.GetLogger().Info("OLS console tls secret reconciled")
	return nil
}

func reconcileConsoleNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateConsoleUINetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateConsolePluginNetworkPolicy, err)
	}
	foundNp := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUINetworkPolicyName, Namespace: r.GetNamespace()}, foundNp)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console NetworkPolicy", "networkpolicy", utils.ConsoleUINetworkPolicyName)
		err = r.Create(ctx, np)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateConsolePluginNetworkPolicy, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetConsolePluginNetworkPolicy, err)
	}
	if utils.NetworkPolicyEqual(np, foundNp) {
		r.GetLogger().Info("Console NetworkPolicy unchanged, reconciliation skipped", "networkpolicy", utils.ConsoleUINetworkPolicyName)
		return nil
	}
	err = r.Update(ctx, np)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateConsolePluginNetworkPolicy, err)
	}
	r.GetLogger().Info("Console NetworkPolicy reconciled", "networkpolicy", utils.ConsoleUINetworkPolicyName)
	return nil

}

// RestartConsoleUI triggers a rolling restart of the Console UI deployment by updating its pod template annotation.
// This is useful when configuration changes require a pod restart (e.g., ConfigMap or Secret updates).
func RestartConsoleUI(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	// If deployment is provided, use it; otherwise fetch it
	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		// Get the Console UI deployment
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: utils.ConsoleUIDeploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			r.GetLogger().Info("failed to get deployment", "deploymentName", utils.ConsoleUIDeploymentName, "error", err)
			return err
		}
	}

	// Initialize annotations map if empty
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}

	// Bump the annotation to trigger a rolling update (new template hash)
	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	// Update the deployment
	r.GetLogger().Info("triggering Console UI rolling restart", "deployment", dep.Name)
	err = r.Update(ctx, dep)
	if err != nil {
		r.GetLogger().Info("failed to update deployment", "deploymentName", dep.Name, "error", err)
		return err
	}

	return nil
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

	// Call Resources phase
	if err := ReconcileConsoleUIResources(r, ctx, olsconfig); err != nil {
		return err
	}

	// Call Deployment phase
	if err := ReconcileConsoleUIDeploymentAndPlugin(r, ctx, olsconfig); err != nil {
		return err
	}

	r.GetLogger().Info("reconcileConsoleUI completed")
	return nil
}
