// Package appserver provides reconciliation logic for the OpenShift Lightspeed application server component.
//
// This package handles the complete lifecycle of the OLS application server, including:
//   - Deployment and pod management
//   - Service account and RBAC configuration
//   - ConfigMap generation for application configuration
//   - Service and networking setup
//   - TLS certificate management
//   - Service monitors and Prometheus rules for observability
//   - Network policies for security
//   - LLM provider secret handling
//
// The main entry point is ReconcileAppServer, which orchestrates all sub-tasks required
// to ensure the application server is running with the correct configuration.
package appserver

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	"sigs.k8s.io/controller-runtime/pkg/client"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileAppServerResources reconciles all resources except the deployment (Phase 1)
// Uses continue-on-error pattern since these resources are independent
func ReconcileAppServerResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileAppServerResources starts")
	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile ServiceAccount",
			Task: reconcileServiceAccount,
		},
		{
			Name: "reconcile SARRole",
			Task: reconcileSARRole,
		},
		{
			Name: "reconcile SARRoleBinding",
			Task: reconcileSARRoleBinding,
		},
		{
			Name: "reconcile OLSConfigMap",
			Task: reconcileOLSConfigMap,
		},
		{
			Name: "reconcile Additional CA ConfigMap",
			Task: reconcileOLSAdditionalCAConfigMap,
		},
		{
			Name: "reconcile Metrics Reader Secret",
			Task: reconcileMetricsReaderSecret,
		},
		{
			Name: "reconcile App NetworkPolicy",
			Task: reconcileAppServerNetworkPolicy,
		},
		{
			Name: "reconcile Proxy CA ConfigMap",
			Task: reconcileProxyCAConfigMap,
		},
	}

	failedTasks := make(map[string]error)

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileAppServerResources error", "task", task.Name)
			failedTasks[task.Name] = err
		}
	}

	if len(failedTasks) > 0 {
		taskNames := make([]string, 0, len(failedTasks))
		for taskName, err := range failedTasks {
			taskNames = append(taskNames, taskName)
			r.GetLogger().Error(err, "Task failed in reconcileAppServerResources", "task", taskName)
		}
		return fmt.Errorf("failed tasks: %v", taskNames)
	}

	r.GetLogger().Info("reconcileAppServerResources completes")
	return nil
}

// ReconcileAppServerDeployment reconciles the deployment and related resources (Phase 2)
func ReconcileAppServerDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileAppServerDeployment starts")

	tasks := []utils.ReconcileTask{
		{
			Name: "reconcile Exporter ConfigMap",
			Task: reconcileExporterConfigMap,
		},
		{
			Name: "reconcile App Deployment",
			Task: reconcileDeployment,
		},
		{
			Name: "reconcile App Service",
			Task: reconcileService,
		},
		{
			Name: "reconcile App TLS Certs",
			Task: ReconcileTLSSecret,
		},
		{
			Name: "reconcile App ServiceMonitor",
			Task: reconcileServiceMonitor,
		},
		{
			Name: "reconcile App PrometheusRule",
			Task: reconcilePrometheusRule,
		},
	}

	for _, task := range tasks {
		err := task.Task(r, ctx, olsconfig)
		if err != nil {
			r.GetLogger().Error(err, "reconcileAppServerDeployment error", "task", task.Name)
			return fmt.Errorf("failed to %s: %w", task.Name, err)
		}
	}

	r.GetLogger().Info("reconcileAppServerDeployment completes")
	return nil
}

func reconcileOLSConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	err := checkLLMCredentials(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrCheckLLMCredentials, err)
	}

	cm, err := GenerateOLSConfigMap(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAPIConfigmap, err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSConfigCmName, Namespace: r.GetNamespace()}, foundCm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIConfigmap, err)
		}
		r.GetStateCache()[utils.OLSConfigHashStateCacheKey] = cm.Annotations[utils.OLSConfigHashKey]
		r.GetStateCache()[utils.PostgresConfigHashStateCacheKey] = cm.Annotations[utils.PostgresConfigHashKey]

		return nil

	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIConfigmap, err)
	}
	foundCmHash, err := utils.HashBytes([]byte(foundCm.Data[utils.OLSConfigFilename]))
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateHash, err)
	}
	// update the state cache with the hash of the existing configmap.
	// so that we can skip the reconciling the deployment if the configmap has not changed.
	r.GetStateCache()[utils.OLSConfigHashStateCacheKey] = cm.Annotations[utils.OLSConfigHashKey]
	r.GetStateCache()[utils.PostgresConfigHashStateCacheKey] = cm.Annotations[utils.PostgresConfigHashKey]
	if foundCmHash == cm.Annotations[utils.OLSConfigHashKey] {
		r.GetLogger().Info("OLS configmap reconciliation skipped", "configmap", foundCm.Name, "hash", foundCm.Annotations[utils.OLSConfigHashKey])
		return nil
	}
	foundCm.Data = cm.Data
	foundCm.Annotations = cm.Annotations
	err = r.Update(ctx, foundCm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAPIConfigmap, err)
	}
	r.GetLogger().Info("OLS configmap reconciled", "configmap", cm.Name, "hash", cm.Annotations[utils.OLSConfigHashKey])
	return nil
}

func reconcileOLSAdditionalCAConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef == nil {
		// no additional CA certs, skip
		r.GetLogger().Info("Additional CA not configured, reconciliation skipped")
		return nil
	}

	// annotate the configmap for watcher
	cm := &corev1.ConfigMap{}

	err := r.Get(ctx, client.ObjectKey{Name: cr.Spec.OLSConfig.AdditionalCAConfigMapRef.Name, Namespace: r.GetNamespace()}, cm)

	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAdditionalCACM, err)
	}

	utils.AnnotateConfigMapWatcher(cm)

	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAdditionalCACM, err)
	}

	certBytes := []byte{}
	for key, value := range cm.Data {
		certBytes = append(certBytes, []byte(key)...)
		certBytes = append(certBytes, []byte(value)...)
	}

	foundCmHash, err := utils.HashBytes(certBytes)
	if err != nil {
		return fmt.Errorf("failed to generate additional CA certs hash %w", err)
	}
	if foundCmHash == r.GetStateCache()[utils.AdditionalCAHashStateCacheKey] {
		r.GetLogger().Info("Additional CA reconciliation skipped", "hash", foundCmHash)
		return nil
	}
	r.GetStateCache()[utils.AdditionalCAHashStateCacheKey] = foundCmHash

	r.GetLogger().Info("additional CA configmap reconciled", "configmap", cm.Name, "hash", foundCmHash)
	return nil
}

func reconcileExporterConfigMap(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	// Check if data collector is enabled
	enabled, err := dataCollectorEnabled(r, cr)
	if err != nil {
		return fmt.Errorf("failed to check if data collector is enabled: %w", err)
	}

	foundCm := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.ExporterConfigCmName, Namespace: r.GetNamespace()}, foundCm)
	cmExists := err == nil

	if !enabled {
		// Data collector is disabled, delete the configmap if it exists
		if cmExists {
			r.GetLogger().Info("deleting exporter configmap", "configmap", utils.ExporterConfigCmName)
			err = r.Delete(ctx, foundCm)
			if err != nil {
				return fmt.Errorf("failed to delete exporter configmap: %w", err)
			}
		} else {
			r.GetLogger().Info("data collector disabled, exporter configmap reconciliation skipped")
		}
		return nil
	}

	// Data collector is enabled, ensure configmap exists
	cm, err := generateExporterConfigMap(r, cr)
	if err != nil {
		return fmt.Errorf("failed to generate exporter configmap: %w", err)
	}

	if !cmExists {
		r.GetLogger().Info("creating exporter configmap", "configmap", cm.Name)
		err = r.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("failed to create exporter configmap: %w", err)
		}
	} else {
		r.GetLogger().Info("exporter configmap already exists, reconciliation skipped", "configmap", cm.Name)
	}

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
	utils.AnnotateConfigMapWatcher(cm)
	err = r.Update(ctx, cm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateProxyCACM, err)
	}

	r.GetLogger().Info("proxy CA configmap reconciled", "configmap", cm.Name)
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
		r.GetLogger().Info("creating a new service account", "serviceAccount", sa.Name)
		err = r.Create(ctx, sa)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIServiceAccount, err)
	}
	r.GetLogger().Info("OLS service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func reconcileSARRole(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	role, err := GenerateSARClusterRole(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateSARClusterRole, err)
	}

	foundRole := &rbacv1.ClusterRole{}
	err = r.Get(ctx, client.ObjectKey{Name: role.Name}, foundRole)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new SAR cluster role", "ClusterRole", role.Name)
		err = r.Create(ctx, role)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateSARClusterRole, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetSARClusterRole, err)
	}
	r.GetLogger().Info("SAR cluster role reconciled", "ClusterRole", role.Name)
	return nil
}

func reconcileSARRoleBinding(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	rb, err := generateSARClusterRoleBinding(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateSARClusterRoleBinding, err)
	}

	foundRB := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, client.ObjectKey{Name: rb.Name}, foundRB)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new SAR cluster role binding", "ClusterRoleBinding", rb.Name)
		err = r.Create(ctx, rb)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateSARClusterRoleBinding, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetSARClusterRoleBinding, err)
	}
	r.GetLogger().Info("SAR cluster role binding reconciled", "ClusterRoleBinding", rb.Name)
	return nil
}

func reconcileDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GenerateOLSDeployment(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAPIDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		utils.UpdateDeploymentAnnotations(desiredDeployment, map[string]string{
			utils.OLSConfigHashKey:      r.GetStateCache()[utils.OLSConfigHashStateCacheKey],
			utils.OLSAppTLSHashKey:      r.GetStateCache()[utils.OLSAppTLSHashStateCacheKey],
			utils.LLMProviderHashKey:    r.GetStateCache()[utils.LLMProviderHashStateCacheKey],
			utils.PostgresSecretHashKey: r.GetStateCache()[utils.PostgresSecretHashStateCacheKey],
		})
		utils.UpdateDeploymentTemplateAnnotations(desiredDeployment, map[string]string{
			utils.OLSConfigHashKey:      r.GetStateCache()[utils.OLSConfigHashStateCacheKey],
			utils.OLSAppTLSHashKey:      r.GetStateCache()[utils.OLSAppTLSHashStateCacheKey],
			utils.LLMProviderHashKey:    r.GetStateCache()[utils.LLMProviderHashStateCacheKey],
			utils.PostgresSecretHashKey: r.GetStateCache()[utils.PostgresSecretHashStateCacheKey],
		})
		r.GetLogger().Info("creating a new deployment", "deployment", desiredDeployment.Name)
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIDeployment, err)
	}

	err = updateOLSDeployment(r, ctx, existingDeployment, desiredDeployment)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAPIDeployment, err)
	}

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
		r.GetLogger().Info("creating a new service", "service", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAPIService, err)
		}

		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAPIServiceAccount, err)
	}

	if utils.ServiceEqual(foundService, service) && foundService.Annotations != nil {
		if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.CAcertificate != "" {
			r.GetLogger().Info("OLS service unchanged, reconciliation skipped", "service", service.Name)
			return nil

		} else if foundService.Annotations[utils.ServingCertSecretAnnotationKey] == service.Annotations[utils.ServingCertSecretAnnotationKey] {
			r.GetLogger().Info("OLS service unchanged, reconciliation skipped", "service", service.Name)
			return nil
		}
	}

	err = r.Update(ctx, service)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAPIService, err)
	}

	r.GetLogger().Info("OLS service reconciled", "service", service.Name)
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
		r.GetLogger().Info("Prometheus Operator not available, skipping app server ServiceMonitor reconciliation")
		return nil
	}

	sm, err := GenerateServiceMonitor(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateServiceMonitor, err)
	}

	foundSm := &monv1.ServiceMonitor{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AppServerServiceMonitorName, Namespace: r.GetNamespace()}, foundSm)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new service monitor", "serviceMonitor", sm.Name)
		err = r.Create(ctx, sm)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateServiceMonitor, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetServiceMonitor, err)
	}
	if utils.ServiceMonitorEqual(foundSm, sm) {
		r.GetLogger().Info("OLS service monitor unchanged, reconciliation skipped", "serviceMonitor", sm.Name)
		return nil
	}
	foundSm.Spec = sm.Spec
	err = r.Update(ctx, foundSm)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateServiceMonitor, err)
	}
	r.GetLogger().Info("OLS service monitor reconciled", "serviceMonitor", sm.Name)
	return nil
}

func reconcilePrometheusRule(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	if !r.IsPrometheusAvailable() {
		r.GetLogger().Info("Prometheus Operator not available, skipping app server PrometheusRule reconciliation")
		return nil
	}

	rule, err := GeneratePrometheusRule(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGeneratePrometheusRule, err)
	}

	foundRule := &monv1.PrometheusRule{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AppServerPrometheusRuleName, Namespace: r.GetNamespace()}, foundRule)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new prometheus rule", "prometheusRule", rule.Name)
		err = r.Create(ctx, rule)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreatePrometheusRule, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetPrometheusRule, err)
	}
	if utils.PrometheusRuleEqual(foundRule, rule) {
		r.GetLogger().Info("OLS prometheus rule unchanged, reconciliation skipped", "prometheusRule", rule.Name)
		return nil
	}
	foundRule.Spec = rule.Spec
	err = r.Update(ctx, foundRule)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateServiceMonitor, err)
	}
	r.GetLogger().Info("OLS prometheus rule reconciled", "prometheusRule", rule.Name)
	return nil
}

func ReconcileTLSSecret(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	foundSecret := &corev1.Secret{}
	var err, lastErr error
	var secretValues map[string]string
	secretName := utils.OLSCertsSecretName
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		secretName = cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name
	}
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, utils.ResourceCreationTimeout, true, func(ctx context.Context) (bool, error) {
		secretValues, err = utils.GetSecretContent(r, secretName, r.GetNamespace(), []string{"tls.key", "tls.crt"}, foundSecret)
		if err != nil {
			lastErr = fmt.Errorf("secret: %s does not have expected tls.key or tls.crt. error: %w", secretName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("%s -%s - wait err %w; last error: %w", utils.ErrGetTLSSecret, utils.OLSCertsSecretName, err, lastErr)
	}

	utils.AnnotateSecretWatcher(foundSecret)
	err = r.Update(ctx, foundSecret)
	if err != nil {
		return fmt.Errorf("failed to update secret:%s. error: %w", foundSecret.Name, err)
	}
	foundTLSSecretHash, err := utils.HashBytes([]byte(secretValues["tls.key"] + secretValues["tls.crt"]))
	if err != nil {
		return fmt.Errorf("failed to generate OLS app TLS certs hash %w", err)
	}
	if foundTLSSecretHash == r.GetStateCache()[utils.OLSAppTLSHashStateCacheKey] {
		r.GetLogger().Info("OLS app TLS secret reconciliation skipped", "hash", foundTLSSecretHash)
		return nil
	}
	r.GetStateCache()[utils.OLSAppTLSHashStateCacheKey] = foundTLSSecretHash
	r.GetLogger().Info("OLS app TLS secret reconciled", "hash", foundTLSSecretHash)
	return nil
}

func reconcileAppServerNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	networkPolicy, err := GenerateAppServerNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAppServerNetworkPolicy, err)
	}

	foundNP := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerNetworkPolicyName, Namespace: r.GetNamespace()}, foundNP)
	if err != nil && errors.IsNotFound(err) {
		r.GetLogger().Info("creating a new network policy", "networkPolicy", networkPolicy.Name)
		err = r.Create(ctx, networkPolicy)
		if err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAppServerNetworkPolicy, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAppServerNetworkPolicy, err)
	}
	if utils.NetworkPolicyEqual(foundNP, networkPolicy) {
		r.GetLogger().Info("OLS app server network policy unchanged, reconciliation skipped", "networkPolicy", networkPolicy.Name)
		return nil
	}
	foundNP.Spec = networkPolicy.Spec
	err = r.Update(ctx, foundNP)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAppServerNetworkPolicy, err)
	}
	r.GetLogger().Info("OLS app server network policy reconciled", "networkPolicy", networkPolicy.Name)
	return nil
}

// =============================================================================
// Test Helper Functions
// =============================================================================
// The following functions are convenience wrappers used primarily by unit tests.
// Production code should call ReconcileAppServerResources and ReconcileAppServerDeployment directly.

// ReconcileAppServer reconciles all app server resources in the original order.
// This function is maintained for backward compatibility with existing tests.
// New code should call ReconcileAppServerResources and ReconcileAppServerDeployment separately.
func ReconcileAppServer(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileAppServer starts")

	// Call Resources phase
	if err := ReconcileAppServerResources(r, ctx, olsconfig); err != nil {
		return err
	}

	// Call Deployment phase
	if err := ReconcileAppServerDeployment(r, ctx, olsconfig); err != nil {
		return err
	}

	r.GetLogger().Info("reconcileAppServer completes")
	return nil
}
