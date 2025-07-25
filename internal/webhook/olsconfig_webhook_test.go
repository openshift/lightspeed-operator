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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("OLSConfig Webhook Validator", Ordered, func() {
	var (
		olsConfig       *olsv1alpha1.OLSConfig
		ctx             context.Context
		validator       *OLSConfigValidator
		openaiSecret    *v1.Secret
		malformedSecret *v1.Secret
		namespace       *v1.Namespace
	)

	BeforeAll(func() {
		validator = &OLSConfigValidator{Client: k8sClient, Namespace: "openshift-lightspeed"}
		ctx = context.Background()

		namespace = &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-lightspeed",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		openaiSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openai-secret",
				Namespace: "openshift-lightspeed",
			},
			Data: map[string][]byte{
				"apitoken": []byte("test-key"),
			},
		}
		Expect(k8sClient.Create(ctx, openaiSecret)).To(Succeed())

		malformedSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "malformed-secret",
				Namespace: "openshift-lightspeed",
			},
			Data: map[string][]byte{
				"notapitoken": []byte("test-key"),
			},
		}
		Expect(k8sClient.Create(ctx, malformedSecret)).To(Succeed())
	})

	BeforeEach(func() {
		olsConfig = &olsv1alpha1.OLSConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
			Spec: olsv1alpha1.OLSConfigSpec{
				LLMConfig: olsv1alpha1.LLMSpec{
					Providers: []olsv1alpha1.ProviderSpec{
						{
							Name: "openai",
							CredentialsSecretRef: v1.LocalObjectReference{
								Name: "openai-secret",
							},
							Models: []olsv1alpha1.ModelSpec{
								{
									Name: "gpt-4o",
								},
							},
						},
					},
				},
			},
		}

	})

	AfterEach(func() {

	})

	AfterAll(func() {
		Expect(k8sClient.Delete(ctx, openaiSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, malformedSecret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Describe("ValidateCreate", func() {
		It("should allow creation of a valid OLSConfig", func() {

			olsConfig.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{
				{
					Name: "openai",
					CredentialsSecretRef: v1.LocalObjectReference{
						Name: "openai-secret",
					},
				},
			}

			warnings, err := validator.ValidateCreate(ctx, olsConfig)
			Expect(err).To(Succeed())
			Expect(warnings).To(BeNil())
		})

		It("should reject creation of an invalid OLSConfig", func() {

			By("reject when the secret is malformed")
			olsConfig.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{
				{
					Name: "openai",
					CredentialsSecretRef: v1.LocalObjectReference{
						Name: "malformed-secret",
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, olsConfig)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())

			By("reject when the secret is inexistent")
			olsConfig.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{
				{
					Name: "openai",
					CredentialsSecretRef: v1.LocalObjectReference{
						Name: "inexistent-secret",
					},
				},
			}
			warnings, err = validator.ValidateCreate(ctx, olsConfig)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())

			By("reject when the olsconfig is not named 'cluster'")
			olsConfig.Name = "not-cluster"
			warnings, err = validator.ValidateCreate(ctx, olsConfig)
			Expect(err).To(HaveOccurred())
			Expect(warnings).To(BeNil())
		})
	})

	Describe("ValidateUpdate", func() {
		var oldConfig *olsv1alpha1.OLSConfig

		BeforeEach(func() {
			oldConfig = olsConfig.DeepCopy()
		})

		It("should allow a valid update", func() {
			oldConfig = olsConfig.DeepCopy()
			olsConfig.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{
				{
					Name: "openai",
					CredentialsSecretRef: v1.LocalObjectReference{
						Name: "openai-secret",
					},
					Models: []olsv1alpha1.ModelSpec{
						{
							Name: "gpt-4o",
						},
						{
							Name: "gpt-4o-mini",
						},
					},
				},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldConfig, olsConfig)
			Expect(err).To(Succeed())
			Expect(warnings).To(BeNil())
		})

		It("should reject an invalid update", func() {
			oldConfig = olsConfig.DeepCopy()
			olsConfig.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{
				{
					Name: "openai",
					CredentialsSecretRef: v1.LocalObjectReference{
						Name: "inexistent-secret",
					},
					Models: []olsv1alpha1.ModelSpec{
						{
							Name: "gpt-4o",
						},
					},
				},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldConfig, olsConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get secret inexistent-secret"))
			Expect(warnings).To(BeNil())
			olsConfig.Spec.LLMConfig.Providers = []olsv1alpha1.ProviderSpec{
				{
					Name: "openai",
					CredentialsSecretRef: v1.LocalObjectReference{
						Name: "malformed-secret",
					},
					Models: []olsv1alpha1.ModelSpec{
						{
							Name: "gpt-4o",
						},
					},
				},
			}
			warnings, err = validator.ValidateUpdate(ctx, oldConfig, olsConfig)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secret malformed-secret does not have the key 'apitoken'"))
			Expect(warnings).To(BeNil())
		})
	})

	Describe("ValidateDelete", func() {
		It("should allow deletion by default", func() {
			warnings, err := validator.ValidateDelete(ctx, olsConfig)
			Expect(err).To(Succeed())
			Expect(warnings).To(BeNil())
		})
	})
})
