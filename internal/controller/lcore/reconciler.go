// Package lcore provides reconciliation logic for the LightSpeed Core (LCore) component.
//
// This package handles the complete lifecycle of the LCore stack, which includes:
//   - Llama Stack for AI model serving
//   - Lightspeed Stack for OLS integration
//   - Deployment and pod management for both containers
//   - Service account and RBAC configuration
//   - ConfigMap generation for both Llama Stack and OLS configuration
//   - Service and networking setup
//   - Service monitors and Prometheus rules for observability
//   - Network policies for security
//
// The main entry point is ReconcileLCore, which orchestrates all sub-tasks required
// to ensure the LCore stack is running with the correct configuration.
package lcore

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// ReconcileLCoreResources reconciles all resources except the deployment (Phase 1)
// Uses continue-on-error pattern since these resources are independent
func ReconcileLCoreResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileLCoreResources starts")
	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile LCore ServiceAccount",
			Task: reconcileServiceAccount,
		},
		{
			Name: "reconcile LCore SARRole",
			Task: reconcileSARRole,
		},
		{
			Name: "reconcile LCore SARRoleBinding",
			Task: reconcileSARRoleBinding,
		},
		{
			Name: "reconcile Llama Stack ConfigMap",
			Task: reconcileLlamaStackConfigMap,
		},
		{
			Name: "reconcile LCore ConfigMap",
			Task: reconcileLcoreConfigMap,
		},
		{
			Name: "reconcile OLS Additional CA ConfigMap",
			Task: reconcileOLSAdditionalCAConfigMap,
		},
		{
			Name: "reconcile Proxy CA ConfigMap",
			Task: reconcileProxyCAConfigMap,
		},
		{
			Name: "reconcile Metrics Reader Secret",
			Task: reconcileMetricsReaderSecret,
		},
		{
			Name: "reconcile LCore NetworkPolicy",
			Task: reconcileNetworkPolicy,
		},
	}

	failedTasks := make(map[string]error)

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileLCoreResources error", "task", task.Name)
			failedTasks[task.Name] = err
		}
	}

	if len(failedTasks) > 0 {
		taskNames := make([]string, 0, len(failedTasks))
		for taskName, err := range failedTasks {
			taskNames = append(taskNames, taskName)
			r.GetLogger().Error(err, "Task failed in reconcileLCoreResources", "task", taskName)
		}
		return fmt.Errorf("failed tasks: %v", taskNames)
	}

	r.GetLogger().Info("reconcileLCoreResources completes")
	return nil
}

// ReconcileLCoreDeployment reconciles the deployment and related resources (Phase 2)
func ReconcileLCoreDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileLCoreDeployment starts")

	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile LCore Deployment",
			Task: reconcileDeployment,
		},
		{
			Name: "reconcile LCore Service",
			Task: reconcileService,
		},
		{
			Name: "reconcile LCore TLS Certs",
			Task: reconcileTLSSecret,
		},
		{
			Name: "reconcile LCore ServiceMonitor",
			Task: reconcileServiceMonitor,
		},
		{
			Name: "reconcile LCore PrometheusRule",
			Task: reconcilePrometheusRule,
		},
	}

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileLCoreDeployment error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("reconcileLCoreDeployment completes")
	return nil
}

func reconcileServiceAccount(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := GenerateServiceAccount(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAPIServiceAccount, err)
	}
	foundSa := &corev1.ServiceAccount{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerServiceAccountName, Namespace: r.GetNamespace()}, foundSa)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new ServiceAccount", "ServiceAccount", sa.Name)
		err = r.Create(ctx, sa)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIServiceAccount, err)
	}
	r.GetLogger().Info("ServiceAccount reconciliation skipped", "ServiceAccount", sa.Name)
	return nil
}

func reconcileSARRole(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cr_role, err := GenerateSARClusterRole(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateSARClusterRole, err)
	}
	foundCr := &rbacv1.ClusterRole{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerSARRoleName}, foundCr)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new ClusterRole", "ClusterRole", cr_role.Name)
		err = r.Create(ctx, cr_role)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateSARClusterRole, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetSARClusterRole, err)
	}
	r.GetLogger().Info("ClusterRole reconciliation skipped", "ClusterRole", cr_role.Name)
	return nil
}

func reconcileSARRoleBinding(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	rb, err := generateSARClusterRoleBinding(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateSARClusterRoleBinding, err)
	}
	foundRb := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerSARRoleBindingName}, foundRb)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new ClusterRoleBinding", "ClusterRoleBinding", rb.Name)
		err = r.Create(ctx, rb)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateSARClusterRoleBinding, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetSARClusterRoleBinding, err)
	}
	r.GetLogger().Info("ClusterRoleBinding reconciliation skipped", "ClusterRoleBinding", rb.Name)
	return nil
}

func reconcileLlamaStackConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateLlamaStackConfigMap(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateLlamaStackConfigMap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.LlamaStackConfigCmName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new Llama Stack ConfigMap", "ConfigMap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateLlamaStackConfigMap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetLlamaStackConfigMap, err)
	}

	if utils.ConfigMapEqual(foundCm, cm) {
		r.GetLogger().Info("Llama Stack ConfigMap reconciliation skipped", "configmap", foundCm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	err = r.Update(ctx, foundCm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateLlamaStackConfigMap, err)
	}

	r.GetLogger().Info("Llama Stack ConfigMap reconciled", "ConfigMap", cm.Name)
	return nil
}

func reconcileLcoreConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	cm, err := GenerateLcoreConfigMap(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAPIConfigmap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.LCoreConfigCmName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new LCore ConfigMap", "ConfigMap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIConfigmap, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIConfigmap, err)
	}

	if utils.ConfigMapEqual(foundCm, cm) {
		r.GetLogger().Info("LCore ConfigMap reconciliation skipped", "configmap", foundCm.Name)
		return nil
	}

	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	err = r.Update(ctx, foundCm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAPIConfigmap, err)
	}

	r.GetLogger().Info("LCore ConfigMap reconciled", "ConfigMap", cm.Name)
	return nil
}

func reconcileOLSAdditionalCAConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef == nil {
		// no additional CA certs, skip
		r.GetLogger().Info("Additional CA not configured, reconciliation skipped")
		return nil
	}

	// Verify the configmap exists (annotation is handled by main controller)
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cr.Spec.OLSConfig.AdditionalCAConfigMapRef.Name, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAdditionalCACM, err)
	}

	r.GetLogger().Info("additional CA configmap reconciled", "configmap", cm.Name)
	return nil
}

func reconcileProxyCAConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	if cr.Spec.OLSConfig.ProxyConfig == nil || cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef == nil {
		// no proxy CA certs, skip
		r.GetLogger().Info("Proxy CA not configured, reconciliation skipped")
		return nil
	}

	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef.Name, Namespace: r.GetNamespace()}, cm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetProxyCACM, err)
	}
	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateProxyCACM, err)
	}

	r.GetLogger().Info("proxy CA configmap reconciled", "configmap", cm.Name)
	return nil
}

func reconcileService(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	service, err := GenerateService(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAPIService, err)
	}
	foundService := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerServiceName, Namespace: r.GetNamespace()}, foundService)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new Service", "Service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIService, err)
		}
		r.GetLogger().Info("Service created", "Service", service.Name)
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIService, err)
	}

	if utils.ServiceEqual(foundService, service) && foundService.Annotations != nil {
		// Check if service-ca annotation matches (or both absent for custom TLS mode)
		if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
			// Custom TLS mode - no service-ca annotation expected
			r.GetLogger().Info("Service reconciliation skipped", "Service", service.Name)
			return nil
		} else if foundService.Annotations[utils.ServingCertSecretAnnotationKey] == service.Annotations[utils.ServingCertSecretAnnotationKey] {
			// Service-ca mode - check annotation matches
			r.GetLogger().Info("Service reconciliation skipped", "Service", service.Name)
			return nil
		}
	}

	err = r.Update(ctx, service)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAPIService, err)
	}

	r.GetLogger().Info("Service reconciled", "Service", service.Name)
	return nil
}

func reconcileDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GenerateLCoreDeployment(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAPIDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: "lightspeed-stack-deployment", Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIDeployment, err)
	}

	err = updateLCoreDeployment(r, ctx, existingDeployment, desiredDeployment)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAPIDeployment, err)
	}

	return nil
}

func reconcileMetricsReaderSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	secret, err := GenerateMetricsReaderSecret(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateMetricsReaderSecret, err)
	}
	foundSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: r.GetNamespace()}, foundSecret)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new metrics reader secret", "secret", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateMetricsReaderSecret, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetMetricsReaderSecret, err)
	}

	if foundSecret.Type != secret.Type || foundSecret.Annotations["kubernetes.io/service-account.name"] != utils.MetricsReaderServiceAccountName {
		foundSecret.Type = secret.Type
		foundSecret.Annotations["kubernetes.io/service-account.name"] = utils.MetricsReaderServiceAccountName
		err = r.Update(ctx, foundSecret)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrUpdateMetricsReaderSecret, err)
		}
	}
	r.GetLogger().Info("OLS metrics reader secret reconciled", "secret", secret.Name)
	return nil
}

func reconcileServiceMonitor(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	if !r.IsPrometheusAvailable() {
		r.GetLogger().Info("Prometheus Operator not available, skipping LCore ServiceMonitor reconciliation")
		return nil
	}

	sm, err := GenerateServiceMonitor(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateServiceMonitor, err)
	}
	foundSm := &monv1.ServiceMonitor{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AppServerServiceMonitorName, Namespace: r.GetNamespace()}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new ServiceMonitor", "ServiceMonitor", sm.Name)
		err = r.Create(ctx, sm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateServiceMonitor, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetServiceMonitor, err)
	}
	if utils.ServiceMonitorEqual(sm, foundSm) {
		r.GetLogger().Info("ServiceMonitor reconciliation skipped", "ServiceMonitor", sm.Name)
		return nil
	}
	foundSm.Spec = sm.Spec
	err = r.Update(ctx, foundSm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateServiceMonitor, err)
	}
	r.GetLogger().Info("ServiceMonitor reconciled", "ServiceMonitor", sm.Name)
	return nil
}

func reconcilePrometheusRule(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	if !r.IsPrometheusAvailable() {
		r.GetLogger().Info("Prometheus Operator not available, skipping LCore PrometheusRule reconciliation")
		return nil
	}

	pr, err := GeneratePrometheusRule(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePrometheusRule, err)
	}
	foundPr := &monv1.PrometheusRule{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AppServerPrometheusRuleName, Namespace: r.GetNamespace()}, foundPr)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new PrometheusRule", "PrometheusRule", pr.Name)
		err = r.Create(ctx, pr)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePrometheusRule, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPrometheusRule, err)
	}
	if utils.PrometheusRuleEqual(pr, foundPr) {
		r.GetLogger().Info("PrometheusRule reconciliation skipped", "PrometheusRule", pr.Name)
		return nil
	}
	foundPr.Spec = pr.Spec
	err = r.Update(ctx, foundPr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdatePrometheusRule, err)
	}
	r.GetLogger().Info("PrometheusRule reconciled", "PrometheusRule", pr.Name)
	return nil
}

func reconcileTLSSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	var lastErr error
	foundSecret := &corev1.Secret{}
	secretName := utils.OLSCertsSecretName
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		secretName = cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name
	}
	err := wait.PollUntilContextTimeout(ctx, 1*time.Second, utils.ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		var getErr error
		_, getErr = utils.GetSecretContent(r, secretName, r.GetNamespace(), []string{"tls.key", "tls.crt"}, foundSecret)
		if getErr != nil {
			lastErr = fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", secretName, getErr)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("%s -%s - wait err %w; last error: %w", utils.ErrGetTLSSecret, utils.OLSCertsSecretName, err, lastErr)
	}

	err = r.Update(ctx, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	r.GetLogger().Info("LCore TLS secret reconciled", "secret", secretName)
	return nil
}

func reconcileNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateAppServerNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAppServerNetworkPolicy, err)
	}
	foundNp := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerNetworkPolicyName, Namespace: r.GetNamespace()}, foundNp)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new NetworkPolicy", "NetworkPolicy", np.Name)
		err = r.Create(ctx, np)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAppServerNetworkPolicy, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAppServerNetworkPolicy, err)
	}
	if utils.NetworkPolicyEqual(np, foundNp) {
		r.GetLogger().Info("NetworkPolicy reconciliation skipped", "NetworkPolicy", np.Name)
		return nil
	}
	foundNp.Spec = np.Spec
	err = r.Update(ctx, foundNp)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAppServerNetworkPolicy, err)
	}
	r.GetLogger().Info("NetworkPolicy reconciled", "NetworkPolicy", np.Name)
	return nil
}

// =============================================================================
// Test Helper Functions
// =============================================================================
// The following functions are convenience wrappers used primarily by unit tests.
// Production code should call ReconcileLCoreResources and ReconcileLCoreDeployment directly.

// ReconcileLCore reconciles all LCore resources in the original order.
// This function is maintained for backward compatibility with existing tests.
// New code should call ReconcileLCoreResources and ReconcileLCoreDeployment separately.
func ReconcileLCore(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileLCore starts")

	// Call Resources phase
	if err := ReconcileLCoreResources(r, ctx, olsconfig); err != nil {
		return err
	}

	// Call Deployment phase
	if err := ReconcileLCoreDeployment(r, ctx, olsconfig); err != nil {
		return err
	}

	r.GetLogger().Info("reconcileLCore completes")
	return nil
}
