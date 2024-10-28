//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *APIContainerConfig) DeepCopyInto(out *APIContainerConfig) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new APIContainerConfig.
func (in *APIContainerConfig) DeepCopy() *APIContainerConfig {
	if in == nil {
		return nil
	}
	out := new(APIContainerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConsoleContainerConfig) DeepCopyInto(out *ConsoleContainerConfig) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConsoleContainerConfig.
func (in *ConsoleContainerConfig) DeepCopy() *ConsoleContainerConfig {
	if in == nil {
		return nil
	}
	out := new(ConsoleContainerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConversationCacheSpec) DeepCopyInto(out *ConversationCacheSpec) {
	*out = *in
	in.Redis.DeepCopyInto(&out.Redis)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConversationCacheSpec.
func (in *ConversationCacheSpec) DeepCopy() *ConversationCacheSpec {
	if in == nil {
		return nil
	}
	out := new(ConversationCacheSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DataCollectorContainerConfig) DeepCopyInto(out *DataCollectorContainerConfig) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DataCollectorContainerConfig.
func (in *DataCollectorContainerConfig) DeepCopy() *DataCollectorContainerConfig {
	if in == nil {
		return nil
	}
	out := new(DataCollectorContainerConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeploymentConfig) DeepCopyInto(out *DeploymentConfig) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.APIContainer.DeepCopyInto(&out.APIContainer)
	in.DataCollectorContainer.DeepCopyInto(&out.DataCollectorContainer)
	in.ConsoleContainer.DeepCopyInto(&out.ConsoleContainer)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeploymentConfig.
func (in *DeploymentConfig) DeepCopy() *DeploymentConfig {
	if in == nil {
		return nil
	}
	out := new(DeploymentConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LLMSpec) DeepCopyInto(out *LLMSpec) {
	*out = *in
	if in.Providers != nil {
		in, out := &in.Providers, &out.Providers
		*out = make([]ProviderSpec, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LLMSpec.
func (in *LLMSpec) DeepCopy() *LLMSpec {
	if in == nil {
		return nil
	}
	out := new(LLMSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelParametersSpec) DeepCopyInto(out *ModelParametersSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelParametersSpec.
func (in *ModelParametersSpec) DeepCopy() *ModelParametersSpec {
	if in == nil {
		return nil
	}
	out := new(ModelParametersSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ModelSpec) DeepCopyInto(out *ModelSpec) {
	*out = *in
	out.Parameters = in.Parameters
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ModelSpec.
func (in *ModelSpec) DeepCopy() *ModelSpec {
	if in == nil {
		return nil
	}
	out := new(ModelSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OLSConfig) DeepCopyInto(out *OLSConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OLSConfig.
func (in *OLSConfig) DeepCopy() *OLSConfig {
	if in == nil {
		return nil
	}
	out := new(OLSConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OLSConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OLSConfigList) DeepCopyInto(out *OLSConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]OLSConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OLSConfigList.
func (in *OLSConfigList) DeepCopy() *OLSConfigList {
	if in == nil {
		return nil
	}
	out := new(OLSConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *OLSConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OLSConfigSpec) DeepCopyInto(out *OLSConfigSpec) {
	*out = *in
	in.LLMConfig.DeepCopyInto(&out.LLMConfig)
	in.OLSConfig.DeepCopyInto(&out.OLSConfig)
	out.OLSDataCollectorConfig = in.OLSDataCollectorConfig
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OLSConfigSpec.
func (in *OLSConfigSpec) DeepCopy() *OLSConfigSpec {
	if in == nil {
		return nil
	}
	out := new(OLSConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OLSConfigStatus) DeepCopyInto(out *OLSConfigStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OLSConfigStatus.
func (in *OLSConfigStatus) DeepCopy() *OLSConfigStatus {
	if in == nil {
		return nil
	}
	out := new(OLSConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OLSDataCollectorSpec) DeepCopyInto(out *OLSDataCollectorSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OLSDataCollectorSpec.
func (in *OLSDataCollectorSpec) DeepCopy() *OLSDataCollectorSpec {
	if in == nil {
		return nil
	}
	out := new(OLSDataCollectorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OLSSpec) DeepCopyInto(out *OLSSpec) {
	*out = *in
	in.ConversationCache.DeepCopyInto(&out.ConversationCache)
	in.DeploymentConfig.DeepCopyInto(&out.DeploymentConfig)
	if in.QueryFilters != nil {
		in, out := &in.QueryFilters, &out.QueryFilters
		*out = make([]QueryFiltersSpec, len(*in))
		copy(*out, *in)
	}
	out.UserDataCollection = in.UserDataCollection
	if in.TLSConfig != nil {
		in, out := &in.TLSConfig, &out.TLSConfig
		*out = new(TLSConfig)
		**out = **in
	}
	if in.AdditionalCAConfigMapRef != nil {
		in, out := &in.AdditionalCAConfigMapRef, &out.AdditionalCAConfigMapRef
		*out = new(corev1.LocalObjectReference)
		**out = **in
	}
	if in.TLSSecurityProfile != nil {
		in, out := &in.TLSSecurityProfile, &out.TLSSecurityProfile
		*out = new(configv1.TLSSecurityProfile)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OLSSpec.
func (in *OLSSpec) DeepCopy() *OLSSpec {
	if in == nil {
		return nil
	}
	out := new(OLSSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProviderSpec) DeepCopyInto(out *ProviderSpec) {
	*out = *in
	out.CredentialsSecretRef = in.CredentialsSecretRef
	if in.Models != nil {
		in, out := &in.Models, &out.Models
		*out = make([]ModelSpec, len(*in))
		copy(*out, *in)
	}
	if in.TLSSecurityProfile != nil {
		in, out := &in.TLSSecurityProfile, &out.TLSSecurityProfile
		*out = new(configv1.TLSSecurityProfile)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProviderSpec.
func (in *ProviderSpec) DeepCopy() *ProviderSpec {
	if in == nil {
		return nil
	}
	out := new(ProviderSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *QueryFiltersSpec) DeepCopyInto(out *QueryFiltersSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new QueryFiltersSpec.
func (in *QueryFiltersSpec) DeepCopy() *QueryFiltersSpec {
	if in == nil {
		return nil
	}
	out := new(QueryFiltersSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RedisSpec) DeepCopyInto(out *RedisSpec) {
	*out = *in
	if in.MaxMemory != nil {
		in, out := &in.MaxMemory, &out.MaxMemory
		*out = new(intstr.IntOrString)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RedisSpec.
func (in *RedisSpec) DeepCopy() *RedisSpec {
	if in == nil {
		return nil
	}
	out := new(RedisSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TLSConfig) DeepCopyInto(out *TLSConfig) {
	*out = *in
	out.KeyCertSecretRef = in.KeyCertSecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TLSConfig.
func (in *TLSConfig) DeepCopy() *TLSConfig {
	if in == nil {
		return nil
	}
	out := new(TLSConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UserDataCollectionSpec) DeepCopyInto(out *UserDataCollectionSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UserDataCollectionSpec.
func (in *UserDataCollectionSpec) DeepCopy() *UserDataCollectionSpec {
	if in == nil {
		return nil
	}
	out := new(UserDataCollectionSpec)
	in.DeepCopyInto(out)
	return out
}
