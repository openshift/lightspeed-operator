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

package webhook

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

var webhooklog = logf.Log.WithName("webhook")

func SetupWebhookWithManager(mgr ctrl.Manager, client client.Client, namespace string) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&olsv1alpha1.OLSConfig{}).
		WithValidator(&OLSConfigValidator{Client: client, Namespace: namespace}).
		Complete()
}

//+kubebuilder:webhook:path=/validate-ols-openshift-io-v1alpha1-olsconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=ols.openshift.io,resources=olsconfigs,verbs=create;update,versions=v1alpha1,name=volsconfig.kb.io,admissionReviewVersions=v1

type OLSConfigValidator struct {
	Client    client.Client
	Namespace string
}

var _ webhook.CustomValidator = &OLSConfigValidator{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (v *OLSConfigValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	olsConfig, ok := obj.(*olsv1alpha1.OLSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a OLSConfig object, got %T", obj)
	}
	webhooklog.Info("validate create", "name", olsConfig.Name)

	if olsConfig.Name != "cluster" {
		return nil, fmt.Errorf("name must be 'cluster'")
	}
	for _, provider := range olsConfig.Spec.LLMConfig.Providers {
		if err := v.validateAPITokenSecret(ctx, provider.CredentialsSecretRef.Name, provider.Type); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (v *OLSConfigValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {

	olsConfig, ok := newObj.(*olsv1alpha1.OLSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a OLSConfig object, got %T", newObj)
	}
	webhooklog.Info("validate update", "name", olsConfig.Name)
	for _, provider := range olsConfig.Spec.LLMConfig.Providers {
		if err := v.validateAPITokenSecret(ctx, provider.CredentialsSecretRef.Name, provider.Type); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (v *OLSConfigValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	webhooklog.Info("validate delete")
	// This should be no-op, because we don't verify the deletion of the OLSConfig object
	return nil, nil
}

func (v *OLSConfigValidator) validateAPITokenSecret(ctx context.Context, secretName string, providerType string) error {
	if secretName == "" {
		return fmt.Errorf("secret name is required")
	}
	secret := &corev1.Secret{}
	if err := v.Client.Get(ctx, types.NamespacedName{Namespace: v.Namespace, Name: secretName}, secret); err != nil {
		return fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	if _, ok := secret.Data["apitoken"]; !ok {
		if providerType == "azure_openai" {
			for _, key := range []string{"client_id", "tenant_id", "client_secret"} {
				if _, ok := secret.Data[key]; !ok {
					return fmt.Errorf("Azure OpenAI token secret %s missing key '%s'", secretName, key)
				}
			}
		} else {
			return fmt.Errorf("secret %s does not have the key 'apitoken'", secretName)
		}
	}

	return nil
}
