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

type ReconcileTask struct {
	Name string
	Task func(context.Context, *olsv1alpha1.OLSConfig) error
}

type DeleteTask struct {
	Name string
	Task func(reconciler.Reconciler, context.Context) error
}

func ReconcileConsoleUI(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileConsoleUI starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile Console Plugin ConfigMap",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcileConsoleUIConfigMap(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Console Plugin Service",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcileConsoleUIService(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Console Plugin TLS Certs",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcileConsoleTLSSecret(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Console Plugin Deployment",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return ReconcileConsoleUIDeployment(r, ctx, cr)
			},
		},
		{
			Name: "reconcile Console Plugin",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcileConsoleUIPlugin(r, ctx, cr)
			},
		},
		{
			Name: "activate Console Plugin",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error { return activateConsoleUI(r, ctx, cr) },
		},
		{
			Name: "reconcile Console Plugin NetworkPolicy",
			Task: func(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
				return reconcileConsoleNetworkPolicy(r, ctx, cr)
			},
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileConsoleUI error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("reconcileConsoleUI completed")

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
		utils.UpdateDeploymentAnnotations(deployment, map[string]string{
			utils.OLSConsoleTLSHashKey: r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey],
		})
		utils.UpdateDeploymentTemplateAnnotations(deployment, map[string]string{
			utils.OLSConsoleTLSHashKey: r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey],
		})
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
	if utils.DeploymentSpecEqual(&foundDeployment.Spec, &deployment.Spec) &&
		foundDeployment.Annotations[utils.OLSConsoleTLSHashKey] == r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey] &&
		foundDeployment.Spec.Template.Annotations[utils.OLSConsoleTLSHashKey] == r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey] {
		r.GetLogger().Info("Console UI deployment unchanged, reconciliation skipped", "deployment", deployment.Name)
		return nil
	}

	foundDeployment.Spec = deployment.Spec
	utils.UpdateDeploymentAnnotations(foundDeployment, map[string]string{
		utils.OLSConsoleTLSHashKey: r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey],
	})
	utils.UpdateDeploymentTemplateAnnotations(foundDeployment, map[string]string{
		utils.OLSConsoleTLSHashKey: r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey],
	})
	err = r.Update(ctx, foundDeployment)
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
	tasks := []DeleteTask{
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
	var secretValues map[string]string
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, utils.ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		secretValues, err = utils.GetSecretContent(r, utils.ConsoleUIServiceCertSecretName, r.GetNamespace(), []string{"tls.key", "tls.crt"}, foundSecret)
		if err != nil {
			lastErr = fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", utils.ConsoleUIServiceCertSecretName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to get TLS key and cert - wait err %w; last error: %w", err, lastErr)
	}
	// TODO: Annotate secret for watcher if needed
	// utils.AnnotateSecretWatcher(foundSecret)
	err = r.Update(ctx, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	foundTLSSecretHash, err := utils.HashBytes([]byte(secretValues["tls.key"] + secretValues["tls.crt"]))
	if err != nil {
		return fmt.Errorf("failed to generate OLS console tls certs hash %w", err)
	}
	if foundTLSSecretHash == r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey] {
		r.GetLogger().Info("OLS console tls secret reconciliation skipped", "hash", foundTLSSecretHash)
		return nil
	}
	r.GetStateCache()[utils.OLSConsoleTLSHashStateCacheKey] = foundTLSSecretHash
	r.GetLogger().Info("OLS console tls secret reconciled", "hash", foundTLSSecretHash)
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
