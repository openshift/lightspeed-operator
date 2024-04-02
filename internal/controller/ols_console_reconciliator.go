package controller

import (
	"context"
	"fmt"
	"slices"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func (r *OLSConfigReconciler) reconcileConsoleUI(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.logger.Info("reconcileConsoleUI starts")
	tasks := []ReconcileTask{
		{
			Name: "reconcile Console Plugin ConfigMap",
			Task: r.reconcileConsoleUIConfigMap,
		},
		{
			Name: "reconcile Console Plugin Service",
			Task: r.reconcileConsoleUIService,
		},
		{
			Name: "reconcile Console Plugin TLS Certs",
			Task: r.reconcileConsoleTLSSecret,
		},
		{
			Name: "reconcile Console Plugin Deployment",
			Task: r.reconcileConsoleUIDeployment,
		},
		{
			Name: "reconcile Console Plugin",
			Task: r.reconcileConsoleUIPlugin,
		},
		{
			Name: "activate Console Plugin",
			Task: r.activateConsoleUI,
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx, olsconfig)
		if err != nil {
			r.logger.Error(err, "reconcileConsoleUI error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileConsoleUI completes")

	return nil
}

func (r *OLSConfigReconciler) reconcileConsoleUIConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := r.generateConsoleUIConfigMap(cr)
	if err != nil {
		return fmt.Errorf("failed to generate Console Plugin configmap: %w", err)
	}
	foundCm := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIConfigMapName, Namespace: r.Options.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating Console UI configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create Console UI configmap: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get Console UI configmap: %w", err)
	}

	if apiequality.Semantic.DeepEqual(foundCm.Data, cm.Data) {
		r.logger.Info("Console UI configmap unchanged, reconciliation skipped", "configmap", cm.Name)
		return nil
	}
	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("failed to update Console UI configmap: %w", err)
	}
	r.logger.Info("Console configmap reconciled", "configmap", cm.Name)

	return nil
}

func (r *OLSConfigReconciler) reconcileConsoleUIService(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := r.generateConsoleUIService(cr)
	if err != nil {
		return fmt.Errorf("failed to generate Console Plugin service: %w", err)
	}
	foundService := &corev1.Service{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIServiceName, Namespace: r.Options.Namespace}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating Console UI service", "service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("failed to create Console UI service: %w", err)
		}
		r.logger.Info("Console UI service created", "service", service.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get Console UI service: %w", err)
	}

	if serviceEqual(foundService, service) &&
		foundService.ObjectMeta.Annotations != nil &&
		foundService.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] == service.ObjectMeta.Annotations[ServingCertSecretAnnotationKey] {
		r.logger.Info("Console UI service unchanged, reconciliation skipped", "service", service.Name)
		return nil
	}

	err = r.Update(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to update Console UI service: %w", err)
	}

	r.logger.Info("Console UI service reconciled", "service", service.Name)

	return nil
}

func (r *OLSConfigReconciler) reconcileConsoleUIDeployment(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	deployment, err := r.generateConsoleUIDeployment(cr)
	if err != nil {
		return fmt.Errorf("failed to generate Console Plugin deployment: %w", err)
	}
	foundDeployment := &appsv1.Deployment{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIDeploymentName, Namespace: r.Options.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
		updateDeploymentAnnotations(deployment, map[string]string{
			OLSConsoleTLSHashKey: r.stateCache[OLSConsoleTLSHashStateCacheKey],
		})
		updateDeploymentTemplateAnnotations(deployment, map[string]string{
			OLSConsoleTLSHashKey: r.stateCache[OLSConsoleTLSHashStateCacheKey],
		})
		r.logger.Info("creating Console UI deployment", "deployment", deployment.Name)
		err = r.Create(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to create Console UI deployment: %w", err)
		}
		r.logger.Info("Console UI deployment created", "deployment", deployment.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get Console UI deployment: %w", err)
	}

	// fill in the default values for the deployment for comparison
	SetDefaults_Deployment(deployment)
	if deploymentSpecEqual(&foundDeployment.Spec, &deployment.Spec) && foundDeployment.Annotations[OLSConsoleTLSHashKey] == r.stateCache[OLSConsoleTLSHashStateCacheKey] && foundDeployment.Spec.Template.Annotations[OLSConsoleTLSHashKey] == r.stateCache[OLSConsoleTLSHashStateCacheKey] {
		r.logger.Info("Console UI deployment unchanged, reconciliation skipped", "deployment", deployment.Name)
		return nil
	}

	foundDeployment.Spec = deployment.Spec
	updateDeploymentAnnotations(foundDeployment, map[string]string{
		OLSConsoleTLSHashKey: r.stateCache[OLSConsoleTLSHashStateCacheKey],
	})
	updateDeploymentTemplateAnnotations(foundDeployment, map[string]string{
		OLSConsoleTLSHashKey: r.stateCache[OLSConsoleTLSHashStateCacheKey],
	})
	err = r.Update(ctx, foundDeployment)
	if err != nil {
		return fmt.Errorf("failed to update Console UI deployment: %w", err)
	}
	r.logger.Info("Console UI deployment reconciled", "deployment", deployment.Name)

	return nil
}

func (r *OLSConfigReconciler) reconcileConsoleUIPlugin(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	plugin, err := r.generateConsoleUIPlugin(cr)
	if err != nil {
		return fmt.Errorf("failed to generate Console Plugin: %w", err)
	}
	foundPlugin := &consolev1.ConsolePlugin{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIPluginName}, foundPlugin)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating Console Plugin", "plugin", plugin.Name)
		err = r.Create(ctx, plugin)
		if err != nil {
			return fmt.Errorf("failed to create Console Plugin: %w", err)
		}
		r.logger.Info("Console Plugin created", "plugin", plugin.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get Console Plugin: %w", err)
	}

	if apiequality.Semantic.DeepEqual(foundPlugin.Spec, plugin.Spec) {
		r.logger.Info("Console Plugin unchanged, reconciliation skipped", "plugin", plugin.Name)
		return nil
	}

	foundPlugin.Spec = plugin.Spec
	err = r.Update(ctx, foundPlugin)
	if err != nil {
		return fmt.Errorf("failed to update Console Plugin: %w", err)
	}
	r.logger.Info("Console Plugin reconciled", "plugin", plugin.Name)

	return nil
}

func (r *OLSConfigReconciler) activateConsoleUI(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	console := &openshiftv1.Console{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console)
	if err != nil {
		return fmt.Errorf("failed to get Console: %w", err)
	}
	if console.Spec.Plugins == nil {
		console.Spec.Plugins = []string{ConsoleUIPluginName}
	} else if !slices.Contains(console.Spec.Plugins, ConsoleUIPluginName) {
		console.Spec.Plugins = append(console.Spec.Plugins, ConsoleUIPluginName)
	} else {
		return nil
	}
	err = r.Update(ctx, console)
	if err != nil {
		return fmt.Errorf("failed to update Console: %w", err)
	}
	r.logger.Info("Console UI plugin activated")
	return nil
}

func (r *OLSConfigReconciler) removeConsoleUI(ctx context.Context) error {
	tasks := []DeleteTask{
		{
			Name: "deactivate Console Plugin",
			Task: r.deactivateConsoleUI,
		},
		{
			Name: "delete Console Plugin",
			Task: r.deleteConsoleUIPlugin,
		},
	}

	for _, task := range tasks {
		err := task.Task(ctx)
		if err != nil {
			r.logger.Error(err, "DeleteConsoleUIPlugin error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("DeleteConsoleUIPlugin completes")

	return nil
}

func (r *OLSConfigReconciler) deleteConsoleUIPlugin(ctx context.Context) error {
	plugin := &consolev1.ConsolePlugin{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIPluginName}, plugin)
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Info("Console Plugin not found, skip deletion")
			return nil
		}
		return fmt.Errorf("failed to get Console Plugin: %w", err)
	}
	err = r.Delete(ctx, plugin)
	if err != nil {
		return fmt.Errorf("failed to delete Console Plugin: %w", err)
	}
	r.logger.Info("Console Plugin deleted")
	return nil
}

func (r *OLSConfigReconciler) deactivateConsoleUI(ctx context.Context) error {
	console := &openshiftv1.Console{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: ConsoleCRName}, console)
	if err != nil {
		return fmt.Errorf("failed to get Console: %w", err)
	}
	if console.Spec.Plugins == nil {
		return nil
	}
	if slices.Contains(console.Spec.Plugins, ConsoleUIPluginName) {
		console.Spec.Plugins = slices.DeleteFunc(console.Spec.Plugins, func(name string) bool { return name == ConsoleUIPluginName })
	} else {
		return nil
	}
	err = r.Update(ctx, console)
	if err != nil {
		return fmt.Errorf("failed to update Console: %w", err)
	}
	r.logger.Info("Console UI plugin deactivated")
	return nil
}

func (r *OLSConfigReconciler) reconcileConsoleTLSSecret(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	var secretValues map[string]string
	foundSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIServiceCertSecretName, Namespace: r.Options.Namespace}, foundSecret)
	if err != nil {
		return fmt.Errorf("secret not found: %w", err)
	}
	secretValues, err = getSecretContent(r.Client, ConsoleUIServiceCertSecretName, r.Options.Namespace, []string{"tls.key", "tls.crt"})
	if err != nil {
		return fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", ConsoleUIServiceCertSecretName, err)
	}
	if err = controllerutil.SetControllerReference(cr, foundSecret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference to secret: %s. error: %w", foundSecret.Name, err)
	}
	err = r.Update(ctx, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	foundTLSSecretHash, err := hashBytes([]byte(secretValues["tls.key"] + secretValues["tls.crt"]))
	if err != nil {
		return fmt.Errorf("failed to generate OLS console tls certs hash %w", err)
	}
	if foundTLSSecretHash == r.stateCache[OLSConsoleTLSHashStateCacheKey] {
		r.logger.Info("OLS console tls secret reconciliation skipped", "hash", foundTLSSecretHash)
		return nil
	}
	r.stateCache[OLSConsoleTLSHashStateCacheKey] = foundTLSSecretHash
	r.logger.Info("OLS console tls secret reconciled", "hash", foundTLSSecretHash)
	return nil
}
