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
			r.logger.Error(err, "reconcileAppServer error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.logger.Info("reconcileAppServer completes")

	return nil
}

func (r *OLSConfigReconciler) reconcileConsoleUIConfigMap(ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := r.generateConsoleUIConfigMap(cr)
	if err != nil {
		return fmt.Errorf("failed to generate Console Plugin configmap: %w", err)
	}
	foundCm := &corev1.ConfigMap{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIConfigMapName, Namespace: cr.Namespace}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.logger.Info("creating Console UI configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create Console UI configmap: %w", err)
		}
		r.logger.Info("Console configmap created", "configmap", cm.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get Console UI configmap: %w", err)
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
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIServiceName, Namespace: cr.Namespace}, foundService)
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
	err = r.Update(ctx, service)
	if err != nil {
		return fmt.Errorf("failed to update Console UI service: %w", err)
	}

	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIServiceName, Namespace: cr.Namespace}, foundService)
	if err != nil {
		return fmt.Errorf("failed to get Console UI service: %w", err)
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
	err = r.Client.Get(ctx, client.ObjectKey{Name: ConsoleUIDeploymentName, Namespace: cr.Namespace}, foundDeployment)
	if err != nil && errors.IsNotFound(err) {
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

	if deploymentSpecEqual(&foundDeployment.Spec, &deployment.Spec) {
		r.logger.Info("Console UI deployment unchanged", "deployment", deployment.Name)
		return nil
	}

	err = r.Update(ctx, deployment)
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
		r.logger.Info("Console Plugin unchanged, skip reconciliaiton", "plugin", plugin.Name)
		return nil
	}

	plugin.SetResourceVersion(foundPlugin.GetResourceVersion())
	err = r.Update(ctx, plugin)
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
