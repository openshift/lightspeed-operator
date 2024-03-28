/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/go-logr/logr"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

// Definitions to manage status conditions
const (
	typeApiReady           = "ApiReady"
	typeCacheReady         = "CacheReady"
	typeConsolePluginReady = "ConsolePluginReady"
	typeCRReconciled       = "Reconciled"
)

// OLSConfigReconciler reconciles a OLSConfig object
type OLSConfigReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	logger     logr.Logger
	stateCache map[string]string
	Options    OLSConfigReconcilerOptions
}

type OLSConfigReconcilerOptions struct {
	LightspeedServiceImage      string
	LightspeedServiceRedisImage string
	ConsoleUIImage              string
	Namespace                   string
}

// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ols.openshift.io,resources=olsconfigs/finalizers,verbs=update
// RBAC for managing deployments of OLS application server
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// Service for exposing lightspeed service API endpoints
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// ServiceAccount to run OLS application server
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// ConfigMap for OLS application server configuration
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// Secret access for redis server configuration
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// ConsolePlugin for install console plugin
// +kubebuilder:rbac:groups=console.openshift.io,resources=consolelinks;consoleexternalloglinks;consoleplugins;consoleplugins/finalizers,verbs=get;create;update;delete
// Modify console CR to activate console plugin
// +kubebuilder:rbac:groups=operator.openshift.io,resources=consoles,verbs=watch;list;get;update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=*
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=*
// ServiceMonitor for monitoring OLS application server
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *OLSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// The operator reconciles only for OLSConfig CR with a specific name
	if req.NamespacedName.Name != OLSConfigName {
		r.logger.Info(fmt.Sprintf("Ignoring OLSConfig CR other than %s", OLSConfigName), "name", req.NamespacedName.Name)
		return ctrl.Result{}, nil
	}

	olsconfig := &olsv1alpha1.OLSConfig{}
	err := r.Get(ctx, req.NamespacedName, olsconfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.logger.Info("olsconfig resource not found. Ignoring since object must be deleted")
			err = r.removeConsoleUI(ctx)
			if err != nil {
				r.logger.Error(err, "Failed to remove console UI")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.logger.Error(err, "Failed to get olsconfig")
		return ctrl.Result{}, err
	}
	r.logger.Info("reconciliation starts", "olsconfig generation", olsconfig.Generation)
	err = r.reconcileAppServer(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile application server")
		r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, false, "Failed", nil)
		return ctrl.Result{}, err
	}
	// Update status condition for API server
	r.updateStatusCondition(ctx, olsconfig, typeApiReady, true, "All components are successfully deployed", nil)

	// TODO: Update DB
	// err = r.reconcileRedisServer(ctx, olsconfig)
	// if err != nil {
	// 	r.logger.Error(err, "Failed to reconcile ols redis")
	// 	return ctrl.Result{}, err
	// }
	err = r.reconcileConsoleUI(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile console UI")
		r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, false, "Failed", nil)
		return ctrl.Result{}, err
	}
	// Update status condition for Console Plugin
	r.updateStatusCondition(ctx, olsconfig, typeConsolePluginReady, true, "All components are successfully deployed", nil)

	r.logger.Info("reconciliation done", "olsconfig generation", olsconfig.Generation)

	// Update status condition for Custom Resource
	r.updateStatusCondition(ctx, olsconfig, typeCRReconciled, true, "Custom resource successfully reconciled", nil)

	return ctrl.Result{}, nil
}

// updateStatusCondition updates the status condition of the OLSConfig Custom Resource instance.
// TODO: Should we support Unknown status and ObservedGeneration?
// TODO: conditionType must be metav1.Condition?
func (r *OLSConfigReconciler) updateStatusCondition(ctx context.Context, olsconfig *olsv1alpha1.OLSConfig, conditionType string, status bool, message string, err error) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionUnknown,
		LastTransitionTime: metav1.Time{},
		Reason:             "Reconciling",
	}

	if status {
		condition.Status = metav1.ConditionTrue
	} else {
		condition.Status = metav1.ConditionFalse
	}

	if err != nil {
		condition.Message = fmt.Sprintf("%s: %v", message, err)
	} else {
		condition.Message = message
	}

	meta.SetStatusCondition(&olsconfig.Status.Conditions, condition)

	if updateErr := r.Status().Update(ctx, olsconfig); updateErr != nil {
		r.logger.Error(err, ErrUpdateCRStatusCondition)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *OLSConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.logger = ctrl.Log.WithName("Reconciler")
	r.stateCache = make(map[string]string)

	generationChanged := builder.WithPredicates(predicate.GenerationChangedPredicate{})
	return ctrl.NewControllerManagedBy(mgr).
		For(&olsv1alpha1.OLSConfig{}).
		Owns(&appsv1.Deployment{}, generationChanged).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.ClusterRole{}).
		Owns(&rbacv1.ClusterRoleBinding{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
