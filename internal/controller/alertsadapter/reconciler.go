// Package alertsadapter reconciles the agentic alerts adapter operand that polls
// Alertmanager and creates AgenticRun CRs for firing alerts.
package alertsadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// ReconcileAlertsAdapterResources reconciles Phase 1 alerts adapter resources.
// When configMapRef is unset, operand resources are removed instead.
func ReconcileAlertsAdapterResources(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	if _, ok := utils.AlertsAdapterConfigMapRef(olsconfig); !ok {
		r.GetLogger().Info("alerts adapter disabled; removing operand resources")
		return RemoveAlertsAdapter(r, ctx)
	}

	r.GetLogger().Info("reconcileAlertsAdapterResources starts")

	tasks := []utils.ReconcileTask{
		{Name: "reconcile alerts adapter ServiceAccount", Task: reconcileServiceAccount},
		{Name: "reconcile alerts adapter agenticruns ClusterRole", Task: reconcileAgenticRunsClusterRole},
		{Name: "reconcile alerts adapter agenticruns ClusterRoleBinding", Task: reconcileAgenticRunsClusterRoleBinding},
		{Name: "remove legacy alerts adapter proposals cluster RBAC", Task: removeLegacyProposalsClusterRBAC},
		{Name: "remove legacy alerts adapter config RoleBinding", Task: removeLegacyConfigRoleBinding},
		{Name: "remove legacy alerts adapter config Role", Task: removeLegacyConfigRole},
		{Name: "reconcile alerts adapter Alertmanager RoleBinding", Task: reconcileAlertmanagerRoleBinding},
		{Name: "reconcile alerts adapter NetworkPolicy", Task: reconcileNetworkPolicy},
	}

	failedTasks := make(map[string]error)
	for _, task := range tasks {
		if err := task.Task(r, ctx, olsconfig); err != nil {
			r.GetLogger().Error(err, "reconcileAlertsAdapterResources error", "task", task.Name)
			failedTasks[task.Name] = err
		}
	}

	if len(failedTasks) > 0 {
		taskNames := make([]string, 0, len(failedTasks))
		for taskName := range failedTasks {
			taskNames = append(taskNames, taskName)
		}
		return fmt.Errorf("failed tasks: %v", taskNames)
	}

	r.GetLogger().Info("reconcileAlertsAdapterResources completes")
	return nil
}

// ReconcileAlertsAdapterDeployment reconciles the alerts adapter Deployment (Phase 2).
func ReconcileAlertsAdapterDeployment(r reconciler.Reconciler, ctx context.Context, olsconfig *olsv1alpha1.OLSConfig) error {
	r.GetLogger().Info("reconcileAlertsAdapterDeployment starts")

	if err := reconcileDeployment(r, ctx, olsconfig); err != nil {
		r.GetLogger().Error(err, "reconcileAlertsAdapterDeployment error")
		return fmt.Errorf("failed to reconcile alerts adapter deployment: %w", err)
	}

	r.GetLogger().Info("reconcileAlertsAdapterDeployment completes")
	return nil
}

// RemoveAlertsAdapter deletes all operator-managed alerts adapter resources when the operand
// is disabled (configMapRef unset) or during OLSConfig finalization.
func RemoveAlertsAdapter(r reconciler.Reconciler, ctx context.Context) error {
	tasks := []utils.DeleteTask{
		{Name: "delete alerts adapter deployment", Task: deleteDeployment},
		{Name: "delete alerts adapter network policy", Task: deleteNetworkPolicy},
		{Name: "delete alerts adapter config RoleBinding", Task: deleteConfigRoleBinding},
		{Name: "delete alerts adapter config Role", Task: deleteConfigRole},
		{Name: "delete alerts adapter service account", Task: deleteServiceAccount},
		{Name: "delete alerts adapter Alertmanager RoleBinding", Task: deleteAlertmanagerRoleBinding},
		{Name: "delete alerts adapter agenticruns cluster RBAC", Task: deleteAgenticRunsClusterRBAC},
	}

	var errs []error
	for _, task := range tasks {
		if err := task.Task(r, ctx); err != nil {
			r.GetLogger().Error(err, "RemoveAlertsAdapter error", "task", task.Name)
			errs = append(errs, fmt.Errorf("failed to %s: %w", task.Name, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	r.GetLogger().Info("RemoveAlertsAdapter completed")
	return nil
}

func reconcileServiceAccount(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	sa, err := GenerateServiceAccount(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterServiceAccount, err)
	}

	foundSA := &corev1.ServiceAccount{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterServiceAccountName, Namespace: r.GetNamespace()}, foundSA)
	if err != nil && apierrors.IsNotFound(err) {
		r.GetLogger().Info("creating alerts adapter service account", "serviceAccount", sa.Name)
		if err := r.Create(ctx, sa); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAlertsAdapterServiceAccount, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterServiceAccount, err)
	}

	r.GetLogger().Info("alerts adapter service account reconciled", "serviceAccount", sa.Name)
	return nil
}

func reconcileAgenticRunsClusterRole(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	role, err := GenerateAgenticRunsClusterRole(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterAgenticRunsClusterRole, err)
	}

	foundRole := &rbacv1.ClusterRole{}
	err = r.Get(ctx, client.ObjectKey{Name: role.Name}, foundRole)
	if err != nil && apierrors.IsNotFound(err) {
		r.GetLogger().Info("creating alerts adapter agenticruns cluster role", "ClusterRole", role.Name)
		if err := r.Create(ctx, role); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAlertsAdapterAgenticRunsClusterRole, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterAgenticRunsClusterRole, err)
	}

	r.GetLogger().Info("alerts adapter agenticruns cluster role reconciled", "ClusterRole", role.Name)
	return nil
}

func reconcileAgenticRunsClusterRoleBinding(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	rb, err := GenerateAgenticRunsClusterRoleBinding(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterAgenticRunsClusterRoleBinding, err)
	}

	foundRB := &rbacv1.ClusterRoleBinding{}
	err = r.Get(ctx, client.ObjectKey{Name: rb.Name}, foundRB)
	if err != nil && apierrors.IsNotFound(err) {
		r.GetLogger().Info("creating alerts adapter agenticruns cluster role binding", "ClusterRoleBinding", rb.Name)
		if err := r.Create(ctx, rb); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAlertsAdapterAgenticRunsClusterRoleBinding, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterAgenticRunsClusterRoleBinding, err)
	}

	r.GetLogger().Info("alerts adapter agenticruns cluster role binding reconciled", "ClusterRoleBinding", rb.Name)
	return nil
}

func reconcileAlertmanagerRoleBinding(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	rb, err := GenerateAlertmanagerRoleBinding(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterAlertmanagerRoleBinding, err)
	}

	foundRB := &rbacv1.RoleBinding{}
	err = r.Get(ctx, client.ObjectKey{Name: rb.Name, Namespace: rb.Namespace}, foundRB)
	if err != nil && apierrors.IsNotFound(err) {
		r.GetLogger().Info("creating alerts adapter Alertmanager role binding", "RoleBinding", rb.Name, "namespace", rb.Namespace)
		if err := r.Create(ctx, rb); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAlertsAdapterAlertmanagerRoleBinding, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterAlertmanagerRoleBinding, err)
	}

	r.GetLogger().Info("alerts adapter Alertmanager role binding reconciled", "RoleBinding", rb.Name, "namespace", rb.Namespace)
	return nil
}

func reconcileNetworkPolicy(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	np, err := GenerateNetworkPolicy(r, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterNetworkPolicy, err)
	}

	foundNP := &networkingv1.NetworkPolicy{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterNetworkPolicyName, Namespace: r.GetNamespace()}, foundNP)
	if err != nil && apierrors.IsNotFound(err) {
		r.GetLogger().Info("creating alerts adapter network policy", "networkpolicy", np.Name)
		if err := r.Create(ctx, np); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAlertsAdapterNetworkPolicy, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterNetworkPolicy, err)
	}

	if utils.NetworkPolicyEqual(np, foundNP) {
		r.GetLogger().Info("alerts adapter network policy unchanged, reconciliation skipped", "networkpolicy", np.Name)
		return nil
	}

	foundNP.Labels = np.Labels
	foundNP.Spec = np.Spec
	if err := r.Update(ctx, foundNP); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAlertsAdapterNetworkPolicy, err)
	}

	r.GetLogger().Info("alerts adapter network policy reconciled", "networkpolicy", np.Name)
	return nil
}

func reconcileDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) error {
	desiredDeployment, err := GenerateDeployment(r, ctx, cr)
	if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGenerateAlertsAdapterDeployment, err)
	}

	existingDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterDeploymentName, Namespace: r.GetNamespace()}, existingDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		r.GetLogger().Info("creating alerts adapter deployment", "deployment", desiredDeployment.Name)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			return fmt.Errorf("%s: %w", utils.ErrCreateAlertsAdapterDeployment, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterDeployment, err)
	}

	utils.SetDefaults_Deployment(desiredDeployment)
	if utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec, true) {
		r.GetLogger().Info("alerts adapter deployment unchanged, reconciliation skipped", "deployment", desiredDeployment.Name)
		return nil
	}

	existingDeployment.Spec = desiredDeployment.Spec
	r.GetLogger().Info("updating alerts adapter deployment", "deployment", existingDeployment.Name)
	if err := RestartAlertsAdapter(r, ctx, existingDeployment); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateAlertsAdapterDeployment, err)
	}

	r.GetLogger().Info("alerts adapter deployment reconciled", "deployment", desiredDeployment.Name)
	return nil
}

func deleteDeployment(r reconciler.Reconciler, ctx context.Context) error {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterDeploymentName, Namespace: r.GetNamespace()}, dep)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter deployment not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterDeployment, err)
	}

	if err := r.Delete(ctx, dep); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter deployment: %w", err)
	}

	r.GetLogger().Info("alerts adapter deployment deleted")
	return nil
}

func deleteNetworkPolicy(r reconciler.Reconciler, ctx context.Context) error {
	np := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterNetworkPolicyName, Namespace: r.GetNamespace()}, np)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter network policy not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterNetworkPolicy, err)
	}

	if err := r.Delete(ctx, np); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter network policy: %w", err)
	}

	r.GetLogger().Info("alerts adapter network policy deleted")
	return nil
}

func removeLegacyConfigRoleBinding(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return deleteConfigRoleBinding(r, ctx)
}

func removeLegacyConfigRole(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	return deleteConfigRole(r, ctx)
}

func deleteConfigRoleBinding(r reconciler.Reconciler, ctx context.Context) error {
	rb := &rbacv1.RoleBinding{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterConfigRoleBindingName, Namespace: r.GetNamespace()}, rb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter config RoleBinding not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterConfigRoleBinding, err)
	}

	if err := r.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter config RoleBinding: %w", err)
	}

	r.GetLogger().Info("alerts adapter config RoleBinding deleted")
	return nil
}

func deleteConfigRole(r reconciler.Reconciler, ctx context.Context) error {
	role := &rbacv1.Role{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterConfigRoleName, Namespace: r.GetNamespace()}, role)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter config Role not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterConfigRole, err)
	}

	if err := r.Delete(ctx, role); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter config Role: %w", err)
	}

	r.GetLogger().Info("alerts adapter config Role deleted")
	return nil
}

func deleteServiceAccount(r reconciler.Reconciler, ctx context.Context) error {
	sa := &corev1.ServiceAccount{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterServiceAccountName, Namespace: r.GetNamespace()}, sa)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter service account not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterServiceAccount, err)
	}

	if err := r.Delete(ctx, sa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter service account: %w", err)
	}

	r.GetLogger().Info("alerts adapter service account deleted")
	return nil
}

func deleteAlertmanagerRoleBinding(r reconciler.Reconciler, ctx context.Context) error {
	rb := &rbacv1.RoleBinding{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      utils.AlertsAdapterAlertmanagerRoleBindingName,
		Namespace: utils.OpenShiftMonitoringNamespace,
	}, rb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter Alertmanager RoleBinding not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterAlertmanagerRoleBinding, err)
	}

	if err := r.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter Alertmanager RoleBinding: %w", err)
	}

	r.GetLogger().Info("alerts adapter Alertmanager RoleBinding deleted")
	return nil
}

// isOpenShiftManagedCRBDeleteDenied reports whether err is the OpenShift admission webhook
// that blocks deleting ClusterRoleBindings whose subjects are ServiceAccounts in openshift-* namespaces.
func isOpenShiftManagedCRBDeleteDenied(err error) bool {
	if err == nil || !apierrors.IsForbidden(err) {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "clusterrolebindings-validation.managed.openshift.io") ||
		(strings.Contains(msg, "Deleting ClusterRoleBinding") && strings.Contains(msg, "is not allowed"))
}

func deleteAgenticRunsClusterRBAC(r reconciler.Reconciler, ctx context.Context) error {
	rb := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterAgenticRunsClusterRoleBindingName}, rb)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter agenticruns ClusterRoleBinding not found, skip deletion")
			return deleteAgenticRunsClusterRole(r, ctx)
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterAgenticRunsClusterRoleBinding, err)
	}

	if err := r.Delete(ctx, rb); err != nil {
		if apierrors.IsNotFound(err) {
			return deleteAgenticRunsClusterRole(r, ctx)
		}
		if isOpenShiftManagedCRBDeleteDenied(err) {
			r.GetLogger().Info(
				"alerts adapter agenticruns ClusterRoleBinding deletion blocked by OpenShift; deleting ClusterRole to remove effective permissions",
				"ClusterRoleBinding", rb.Name,
			)
			return deleteAgenticRunsClusterRole(r, ctx)
		}
		return fmt.Errorf("failed to delete alerts adapter agenticruns ClusterRoleBinding: %w", err)
	}

	r.GetLogger().Info("alerts adapter agenticruns ClusterRoleBinding deleted")
	return deleteAgenticRunsClusterRole(r, ctx)
}

func deleteAgenticRunsClusterRole(r reconciler.Reconciler, ctx context.Context) error {
	role := &rbacv1.ClusterRole{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterAgenticRunsClusterRoleName}, role)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.GetLogger().Info("alerts adapter agenticruns ClusterRole not found, skip deletion")
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrGetAlertsAdapterAgenticRunsClusterRole, err)
	}

	if err := r.Delete(ctx, role); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete alerts adapter agenticruns ClusterRole: %w", err)
	}

	r.GetLogger().Info("alerts adapter agenticruns ClusterRole deleted")
	return nil
}

// removeLegacyProposalsClusterRBAC deletes pre-OLS-3475 ClusterRole/Binding names after Proposal→AgenticRun rename.
func removeLegacyProposalsClusterRBAC(r reconciler.Reconciler, ctx context.Context, _ *olsv1alpha1.OLSConfig) error {
	rb := &rbacv1.ClusterRoleBinding{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterLegacyProposalsClusterRoleName}, rb)
	if err == nil {
		delErr := r.Delete(ctx, rb)
		if delErr != nil && !apierrors.IsNotFound(delErr) && !isOpenShiftManagedCRBDeleteDenied(delErr) {
			return fmt.Errorf("failed to delete legacy alerts adapter proposals ClusterRoleBinding: %w", delErr)
		}
		if delErr == nil {
			r.GetLogger().Info("deleted legacy alerts adapter proposals ClusterRoleBinding", "ClusterRoleBinding", rb.Name)
		}
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get legacy alerts adapter proposals ClusterRoleBinding: %w", err)
	}

	role := &rbacv1.ClusterRole{}
	err = r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterLegacyProposalsClusterRoleName}, role)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get legacy alerts adapter proposals ClusterRole: %w", err)
	}

	if delErr := r.Delete(ctx, role); delErr != nil && !apierrors.IsNotFound(delErr) {
		return fmt.Errorf("failed to delete legacy alerts adapter proposals ClusterRole: %w", delErr)
	}
	r.GetLogger().Info("deleted legacy alerts adapter proposals ClusterRole", "ClusterRole", role.Name)
	return nil
}

// RestartAlertsAdapter triggers a rolling restart of the alerts adapter deployment by updating
// its pod template annotation. Used when the user-managed runtime ConfigMap changes.
func RestartAlertsAdapter(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: utils.AlertsAdapterDeploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			return fmt.Errorf("failed to get deployment %s: %w", utils.AlertsAdapterDeploymentName, err)
		}
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}

	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("triggering alerts adapter rolling restart", "deployment", dep.Name)
	if err := r.Update(ctx, dep); err != nil {
		return fmt.Errorf("failed to update deployment %s: %w", dep.Name, err)
	}

	return nil
}
