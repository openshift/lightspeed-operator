package rhokp

import (
	"context"
	"fmt"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileResources reconciles Phase 1 standalone RHOKP resources (NetworkPolicy).
func ReconcileResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileRHOKPResources", []utils.ReconcileTask{
		{Name: "reconcile RHOKP NetworkPolicy", Task: reconcileNetworkPolicy},
		{Name: "reconcile RHOKP egress NetworkPolicy", Task: reconcileEgressNetworkPolicy},
	}, true)
}

// ReconcileDeployment reconciles Phase 2: Service, TLS material, and Deployment.
func ReconcileDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	return utils.RunReconcileTasks(r, ctx, olsconfig, "reconcileRHOKPDeployment", []utils.ReconcileTask{
		{Name: "reconcile RHOKP Service", Task: reconcileService},
		{Name: "reconcile RHOKP TLS Certs", Task: reconcileTLSSecret},
		{Name: "reconcile RHOKP Deployment", Task: reconcileDeployment},
		{Name: "reconcile RHOKP ServiceMonitor", Task: reconcileServiceMonitor},
	}, false)
}

// Remove deletes all operator-managed standalone RHOKP resources.
func Remove(r reconciler.Reconciler, ctx context.Context) error {
	return utils.RunDeleteTasks(r, ctx, "RemoveRHOKP", []utils.DeleteTask{
		{Name: "delete RHOKP deployment", Task: deleteDeployment},
		{Name: "delete RHOKP service", Task: deleteService},
		{Name: "delete RHOKP network policy", Task: deleteNetworkPolicy},
		{Name: "delete RHOKP egress network policy", Task: deleteEgressNetworkPolicy},
		{Name: "delete RHOKP TLS secret", Task: deleteTLSSecret},
		{Name: "delete RHOKP ServiceMonitor", Task: deleteServiceMonitor},
	})
}

func reconcileNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateRHOKPNetworkPolicy, err)
	}

	return utils.ReconcileNetworkPolicy(r, ctx, np)
}

func reconcileEgressNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	ingress, err := GenerateNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateRHOKPNetworkPolicy, err)
	}
	return utils.ReconcileNetworkPolicy(r, ctx, utils.GenerateDenyEgressNetworkPolicy(ingress))
}

func reconcileService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateRHOKPService, err)
	}
	return utils.ReconcileConsolePluginService(r, ctx, service)
}

func reconcileTLSSecret(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return utils.WaitForConsolePluginTLSSecret(r, ctx, utils.RHOKPCertsSecretName)
}

func reconcileDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GenerateDeployment(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateRHOKPDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.RHOKPDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating RHOKP deployment", "deployment", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateRHOKPDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetRHOKPDeployment, err)
	}

	if err := UpdateDeployment(r, ctx, existingDeployment, desiredDeployment); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateRHOKPDeployment, err)
	}

	r.GetLogger().Info("RHOKP deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func deleteDeployment(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &appsv1.Deployment{}, utils.RHOKPDeploymentName)
}

func deleteService(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &corev1.Service{}, utils.RHOKPServiceName)
}

func deleteNetworkPolicy(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &networkingv1.NetworkPolicy{}, utils.RHOKPNetworkPolicyName)
}

func deleteEgressNetworkPolicy(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &networkingv1.NetworkPolicy{}, utils.RHOKPNetworkPolicyName+"-egress")
}

func deleteTLSSecret(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &corev1.Secret{}, utils.RHOKPCertsSecretName)
}

func reconcileServiceMonitor(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sm, err := generateServiceMonitor(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateRHOKPServiceMonitor, err)
	}
	return utils.ReconcileServiceMonitor(r, ctx, sm)
}

func deleteServiceMonitor(r reconciler.Reconciler, ctx context.Context) error {
	return deleteNamespacedObject(r, ctx, &monv1.ServiceMonitor{}, utils.RHOKPServiceMonitorName)
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
