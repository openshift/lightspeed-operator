package utils

import (
	"context"
	stderrors "errors"
	"fmt"
	"slices"
	"time"

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
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
)

// RunReconcileTasks runs reconcile tasks. When continueOnError is true, all tasks run and failures are aggregated.
func RunReconcileTasks(
	r reconciler.Reconciler,
	ctx context.Context,
	cr *olsv1alpha1.OLSConfig,
	phase string,
	tasks []ReconcileTask,
	continueOnError bool,
) error {
	r.GetLogger().Info(phase + " starts")

	if continueOnError {
		var failedErrs []error
		for _, task := range tasks {
			if err := task.Task(r, ctx, cr); err != nil {
				r.GetLogger().Error(err, phase+" error", "task", task.Name)
				failedErrs = append(failedErrs, fmt.Errorf("%s: %w", task.Name, err))
			}
		}
		if len(failedErrs) > 0 {
			return fmt.Errorf("%s: %w", phase, stderrors.Join(failedErrs...))
		}
		r.GetLogger().Info(phase + " completes")
		return nil
	}

	for _, task := range tasks {
		if err := task.Task(r, ctx, cr); err != nil {
			r.GetLogger().Error(err, phase+" error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info(phase + " completes")
	return nil
}

// RunDeleteTasks runs delete tasks and fails fast on the first error.
func RunDeleteTasks(r reconciler.Reconciler, ctx context.Context, phase string, tasks []DeleteTask) error {
	for _, task := range tasks {
		if err := task.Task(r, ctx); err != nil {
			r.GetLogger().Error(err, phase+" error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}
	r.GetLogger().Info(phase + " completed")
	return nil
}

// ReconcileConsolePluginConfigMap creates or updates a console plugin nginx ConfigMap.
func ReconcileConsolePluginConfigMap(r reconciler.Reconciler, ctx context.Context, desired *corev1.ConfigMap) error {
	foundCm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console Plugin configmap", "configmap", desired.Name)
		if err = r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateConsolePluginConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetConsolePluginConfigMap, err)
	}

	if apiequality.Semantic.DeepEqual(foundCm.Data, desired.Data) {
		r.GetLogger().Info("Console Plugin configmap unchanged, reconciliation skipped", "configmap", desired.Name)
		return nil
	}

	foundCm.Data = desired.Data
	if err = r.Update(ctx, foundCm); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsolePluginConfigMap, err)
	}
	r.GetLogger().Info("Console Plugin configmap reconciled", "configmap", foundCm.Name)
	return nil
}

// ReconcileConsolePluginService creates or updates a console plugin Service.
func ReconcileConsolePluginService(r reconciler.Reconciler, ctx context.Context, desired *corev1.Service) error {
	foundService := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: r.GetNamespace()}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console Plugin service", "service", desired.Name)
		if err = r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateConsolePluginService, err)
		}
		r.GetLogger().Info("Console Plugin service created", "service", desired.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetConsolePluginService, err)
	}

	if ServiceEqual(foundService, desired) &&
		foundService.Annotations != nil &&
		foundService.Annotations[ServingCertSecretAnnotationKey] == desired.Annotations[ServingCertSecretAnnotationKey] {
		r.GetLogger().Info("Console Plugin service unchanged, reconciliation skipped", "service", desired.Name)
		return nil
	}

	foundService.Spec = desired.Spec
	foundService.Annotations = desired.Annotations
	foundService.Labels = desired.Labels
	if err = r.Update(ctx, foundService); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsolePluginService, err)
	}
	r.GetLogger().Info("Console Plugin service reconciled", "service", foundService.Name)
	return nil
}

// ReconcileConsolePluginDeployment creates or updates a console plugin Deployment.
func ReconcileConsolePluginDeployment(
	r reconciler.Reconciler,
	ctx context.Context,
	desired *appsv1.Deployment,
	restart func(reconciler.Reconciler, context.Context, ...*appsv1.Deployment) error,
) error {
	foundDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: r.GetNamespace()}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console Plugin deployment", "deployment", desired.Name)
		if err = r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateConsolePluginDeployment, err)
		}
		r.GetLogger().Info("Console Plugin deployment created", "deployment", desired.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetConsolePluginDeployment, err)
	}

	SetDefaults_Deployment(desired)
	if DeploymentSpecEqual(&foundDeployment.Spec, &desired.Spec, true) {
		return nil
	}

	foundDeployment.Spec = desired.Spec
	r.GetLogger().Info("Updating Console Plugin deployment", "deployment", foundDeployment.Name)
	if err = restart(r, ctx, foundDeployment); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsolePluginDeployment, err)
	}
	r.GetLogger().Info("Console Plugin deployment reconciled", "deployment", desired.Name)
	return nil
}

// ReconcileConsolePluginCR creates or updates a ConsolePlugin CR.
func ReconcileConsolePluginCR(r reconciler.Reconciler, ctx context.Context, desired *consolev1.ConsolePlugin) error {
	foundPlugin := &consolev1.ConsolePlugin{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name}, foundPlugin)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console Plugin", "plugin", desired.Name)
		if err = r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateConsolePlugin, err)
		}
		r.GetLogger().Info("Console Plugin created", "plugin", desired.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetConsolePlugin, err)
	}

	if apiequality.Semantic.DeepEqual(foundPlugin.Spec, desired.Spec) {
		r.GetLogger().Info("Console Plugin unchanged, reconciliation skipped", "plugin", desired.Name)
		return nil
	}

	foundPlugin.Spec = desired.Spec
	if err = r.Update(ctx, foundPlugin); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsolePlugin, err)
	}
	r.GetLogger().Info("Console Plugin reconciled", "plugin", desired.Name)
	return nil
}

// ActivateConsolePlugin adds a plugin name to the cluster Console CR.
func ActivateConsolePlugin(r reconciler.Reconciler, ctx context.Context, pluginName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		console := &openshiftv1.Console{}
		if err := r.Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console); err != nil {
			return fmt.Errorf("%s: %w", ErrGetConsole, err)
		}
		if console.Spec.Plugins == nil {
			console.Spec.Plugins = []string{pluginName}
		} else if !slices.Contains(console.Spec.Plugins, pluginName) {
			console.Spec.Plugins = append(console.Spec.Plugins, pluginName)
		} else {
			return nil
		}
		return r.Update(ctx, console)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsole, err)
	}
	r.GetLogger().Info("Console Plugin activated", "plugin", pluginName)
	return nil
}

// DeactivateConsolePlugin removes a plugin name from the cluster Console CR.
func DeactivateConsolePlugin(r reconciler.Reconciler, ctx context.Context, pluginName string) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		console := &openshiftv1.Console{}
		if err := r.Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console); err != nil {
			if errors.IsNotFound(err) {
				r.GetLogger().Info("Console CR not found, skipping plugin deactivation")
				return nil
			}
			return fmt.Errorf("%s: %w", ErrGetConsole, err)
		}
		if console.Spec.Plugins == nil {
			return nil
		}
		if slices.Contains(console.Spec.Plugins, pluginName) {
			console.Spec.Plugins = slices.DeleteFunc(console.Spec.Plugins, func(name string) bool { return name == pluginName })
		} else {
			return nil
		}
		return r.Update(ctx, console)
	})
	if err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsole, err)
	}
	r.GetLogger().Info("Console Plugin deactivated", "plugin", pluginName)
	return nil
}

// DeleteConsolePluginCR deletes a ConsolePlugin CR.
func DeleteConsolePluginCR(r reconciler.Reconciler, ctx context.Context, pluginName string) error {
	plugin := &consolev1.ConsolePlugin{}
	err := r.Get(ctx, client.ObjectKey{Name: pluginName}, plugin)
	if err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("Console Plugin not found, skip deletion", "plugin", pluginName)
			return nil
		}
		return fmt.Errorf("%s: %w", ErrGetConsolePlugin, err)
	}
	if err = r.Delete(ctx, plugin); err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("Console Plugin not found, consider deletion successful", "plugin", pluginName)
			return nil
		}
		return fmt.Errorf("%s: %w", ErrDeleteConsolePlugin, err)
	}
	r.GetLogger().Info("Console Plugin deleted", "plugin", pluginName)
	return nil
}

// WaitForConsolePluginTLSSecret waits for the service-ca TLS secret for a console plugin.
func WaitForConsolePluginTLSSecret(r reconciler.Reconciler, ctx context.Context, secretName string) error {
	foundSecret := &corev1.Secret{}
	var lastErr error
	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		_, err := GetSecretContent(r, ctx, secretName, r.GetNamespace(), []string{"tls.key", "tls.crt"}, foundSecret)
		if err != nil {
			lastErr = fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", secretName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to get TLS key and cert - wait err %w; last error: %w", err, lastErr)
	}
	r.GetLogger().Info("Console Plugin tls secret reconciled", "secret", secretName)
	return nil
}

// ReconcileConsolePluginNetworkPolicy creates or updates a console plugin NetworkPolicy.
func ReconcileConsolePluginNetworkPolicy(r reconciler.Reconciler, ctx context.Context, desired *networkingv1.NetworkPolicy) error {
	foundNp := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: r.GetNamespace()}, foundNp)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating Console Plugin NetworkPolicy", "networkpolicy", desired.Name)
		if err = r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateConsolePluginNetworkPolicy, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", ErrGetConsolePluginNetworkPolicy, err)
	}
	if NetworkPolicyEqual(desired, foundNp) {
		r.GetLogger().Info("Console Plugin NetworkPolicy unchanged, reconciliation skipped", "networkpolicy", desired.Name)
		return nil
	}
	foundNp.Spec = desired.Spec
	if err = r.Update(ctx, foundNp); err != nil {
		return fmt.Errorf("%s: %w", ErrUpdateConsolePluginNetworkPolicy, err)
	}
	r.GetLogger().Info("Console Plugin NetworkPolicy reconciled", "networkpolicy", desired.Name)
	return nil
}

// ReconcileConsolePluginServiceAccount creates a console plugin ServiceAccount if missing.
func ReconcileConsolePluginServiceAccount(r reconciler.Reconciler, ctx context.Context, desired *corev1.ServiceAccount) error {
	foundSa := &corev1.ServiceAccount{}
	err := r.Get(ctx, client.ObjectKey{Name: desired.Name, Namespace: r.GetNamespace()}, foundSa)
	if err != nil && errors.IsNotFound(err) {
		if err = r.Create(ctx, desired); err != nil {
			return fmt.Errorf("%s: %w", ErrCreateConsolePluginServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", ErrGetConsolePluginServiceAccount, err)
	}
	return nil
}

// RestartConsolePluginDeployment triggers a rolling restart of a console plugin deployment.
func RestartConsolePluginDeployment(r reconciler.Reconciler, ctx context.Context, deploymentName string, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			return fmt.Errorf("failed to get deployment %s: %w", deploymentName, err)
		}
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations[ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("triggering Console Plugin rolling restart", "deployment", dep.Name)
	if err = r.Update(ctx, dep); err != nil {
		return fmt.Errorf("failed to update deployment %s: %w", dep.Name, err)
	}
	return nil
}

// RemoveConsolePlugin deactivates and deletes a console plugin from the OpenShift Console.
func RemoveConsolePlugin(r reconciler.Reconciler, ctx context.Context, pluginName string) error {
	return RunDeleteTasks(r, ctx, "RemoveConsolePlugin", []DeleteTask{
		{
			Name: "deactivate Console Plugin",
			Task: func(r reconciler.Reconciler, ctx context.Context) error {
				return DeactivateConsolePlugin(r, ctx, pluginName)
			},
		},
		{
			Name: "delete Console Plugin",
			Task: func(r reconciler.Reconciler, ctx context.Context) error {
				return DeleteConsolePluginCR(r, ctx, pluginName)
			},
		},
	})
}
