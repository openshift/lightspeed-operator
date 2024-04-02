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
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
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
		if errors.IsNotFound(err) {
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
		return ctrl.Result{}, err
	}
	// TODO: Update DB
	// err = r.reconcileRedisServer(ctx, olsconfig)
	// if err != nil {
	// 	r.logger.Error(err, "Failed to reconcile ols redis")
	// 	return ctrl.Result{}, err
	// }
	err = r.reconcileConsoleUI(ctx, olsconfig)
	if err != nil {
		r.logger.Error(err, "Failed to reconcile console UI")
		return ctrl.Result{}, err
	}

	r.logger.Info("reconciliation done", "olsconfig generation", olsconfig.Generation)

	return ctrl.Result{}, nil
}

// waitForSecret waits for the secret to be created before the timeout expires.
func (r *OLSConfigReconciler) waitForSecret(ctx context.Context, name string) error {
	interval := 1 * time.Second
	timeout := 5 * time.Second

	deadlineCtx, deadlineCancel := context.WithTimeout(ctx, timeout)
	defer deadlineCancel()

	secret := corev1.Secret{}
	err := wait.PollUntilContextCancel(deadlineCtx, interval, true, func(ctx context.Context) (bool, error) {
		err := r.Get(ctx, client.ObjectKey{Namespace: r.Options.Namespace, Name: name}, &secret)
		if err != nil && errors.IsNotFound(err) {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return err
	}

	return nil
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
