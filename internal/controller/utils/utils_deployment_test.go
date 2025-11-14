package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("Deployment Manipulation Functions", func() {
	var deployment *appsv1.Deployment

	BeforeEach(func() {
		replicas := int32(1)
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-deployment",
				Namespace: "test-namespace",
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "app-container",
								Image: "myapp:v1",
								Env: []corev1.EnvVar{
									{Name: "ENV1", Value: "value1"},
								},
								VolumeMounts: []corev1.VolumeMount{
									{Name: "vol1", MountPath: "/data"},
								},
								Resources: corev1.ResourceRequirements{},
							},
							{
								Name:  "sidecar-container",
								Image: "sidecar:v1",
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "vol1",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
			},
		}
	})

	Describe("UpdateDeploymentAnnotations", func() {
		It("should add annotations to deployment", func() {
			annotations := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			UpdateDeploymentAnnotations(deployment, annotations)

			Expect(deployment.Annotations).To(HaveLen(2))
			Expect(deployment.Annotations["key1"]).To(Equal("value1"))
			Expect(deployment.Annotations["key2"]).To(Equal("value2"))
		})

		It("should update existing annotations", func() {
			deployment.Annotations = map[string]string{"existing": "old"}
			annotations := map[string]string{"existing": "new", "key1": "value1"}

			UpdateDeploymentAnnotations(deployment, annotations)

			Expect(deployment.Annotations).To(HaveLen(2))
			Expect(deployment.Annotations["existing"]).To(Equal("new"))
			Expect(deployment.Annotations["key1"]).To(Equal("value1"))
		})

		It("should handle nil annotations map", func() {
			deployment.Annotations = nil
			annotations := map[string]string{"key1": "value1"}

			UpdateDeploymentAnnotations(deployment, annotations)

			Expect(deployment.Annotations).NotTo(BeNil())
			Expect(deployment.Annotations["key1"]).To(Equal("value1"))
		})

		It("should handle empty annotations input", func() {
			deployment.Annotations = map[string]string{"existing": "value"}
			UpdateDeploymentAnnotations(deployment, map[string]string{})

			Expect(deployment.Annotations).To(HaveLen(1))
			Expect(deployment.Annotations["existing"]).To(Equal("value"))
		})
	})

	Describe("UpdateDeploymentTemplateAnnotations", func() {
		It("should add annotations to pod template", func() {
			annotations := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}
			UpdateDeploymentTemplateAnnotations(deployment, annotations)

			Expect(deployment.Spec.Template.Annotations).To(HaveLen(2))
			Expect(deployment.Spec.Template.Annotations["key1"]).To(Equal("value1"))
			Expect(deployment.Spec.Template.Annotations["key2"]).To(Equal("value2"))
		})

		It("should update existing template annotations", func() {
			deployment.Spec.Template.Annotations = map[string]string{"existing": "old"}
			annotations := map[string]string{"existing": "new"}

			UpdateDeploymentTemplateAnnotations(deployment, annotations)

			Expect(deployment.Spec.Template.Annotations["existing"]).To(Equal("new"))
		})

		It("should handle nil template annotations", func() {
			deployment.Spec.Template.Annotations = nil
			annotations := map[string]string{"key1": "value1"}

			UpdateDeploymentTemplateAnnotations(deployment, annotations)

			Expect(deployment.Spec.Template.Annotations).NotTo(BeNil())
			Expect(deployment.Spec.Template.Annotations["key1"]).To(Equal("value1"))
		})
	})

	Describe("SetDeploymentReplicas", func() {
		It("should update replicas when different", func() {
			changed := SetDeploymentReplicas(deployment, 3)

			Expect(changed).To(BeTrue())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should not update when replicas are the same", func() {
			changed := SetDeploymentReplicas(deployment, 1)

			Expect(changed).To(BeFalse())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should handle zero replicas", func() {
			changed := SetDeploymentReplicas(deployment, 0)

			Expect(changed).To(BeTrue())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(0)))
		})
	})

	Describe("SetTolerations", func() {
		It("should set tolerations when different", func() {
			tolerations := []corev1.Toleration{
				{
					Key:      "key1",
					Operator: corev1.TolerationOpEqual,
					Value:    "value1",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			}
			changed := SetTolerations(deployment, tolerations)

			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(tolerations))
		})

		It("should not update when tolerations are the same", func() {
			tolerations := []corev1.Toleration{
				{Key: "key1", Operator: corev1.TolerationOpEqual, Value: "value1"},
			}
			deployment.Spec.Template.Spec.Tolerations = tolerations

			changed := SetTolerations(deployment, tolerations)

			Expect(changed).To(BeFalse())
		})

		It("should handle empty tolerations", func() {
			deployment.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Key: "key1"}}
			changed := SetTolerations(deployment, []corev1.Toleration{})

			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Tolerations).To(BeEmpty())
		})
	})

	Describe("SetNodeSelector", func() {
		It("should set node selector when different", func() {
			nodeSelector := map[string]string{
				"disktype": "ssd",
				"region":   "us-west",
			}
			changed := SetNodeSelector(deployment, nodeSelector)

			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(nodeSelector))
		})

		It("should not update when node selector is the same", func() {
			nodeSelector := map[string]string{"disktype": "ssd"}
			deployment.Spec.Template.Spec.NodeSelector = nodeSelector

			changed := SetNodeSelector(deployment, nodeSelector)

			Expect(changed).To(BeFalse())
		})

		It("should handle empty node selector", func() {
			deployment.Spec.Template.Spec.NodeSelector = map[string]string{"key": "value"}
			changed := SetNodeSelector(deployment, map[string]string{})

			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(BeEmpty())
		})
	})

	Describe("SetVolumes", func() {
		It("should set volumes when different", func() {
			newVolumes := []corev1.Volume{
				{
					Name: "vol2",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "config"},
						},
					},
				},
			}
			changed := SetVolumes(deployment, newVolumes)

			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Volumes).To(Equal(newVolumes))
		})

		It("should not update when volumes are the same", func() {
			existingVolumes := deployment.Spec.Template.Spec.Volumes
			changed := SetVolumes(deployment, existingVolumes)

			Expect(changed).To(BeFalse())
		})

		It("should handle volume order differences", func() {
			volumes := []corev1.Volume{
				{Name: "vol-b", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "vol-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			}
			deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
				{Name: "vol-a", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "vol-b", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			}

			changed := SetVolumes(deployment, volumes)

			// Should be false because after sorting they're the same
			Expect(changed).To(BeFalse())
		})
	})

	Describe("SetVolumeMounts", func() {
		It("should set volume mounts when different", func() {
			newMounts := []corev1.VolumeMount{
				{Name: "vol2", MountPath: "/config"},
			}
			changed, err := SetVolumeMounts(deployment, newMounts, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(Equal(newMounts))
		})

		It("should not update when volume mounts are the same", func() {
			existingMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
			changed, err := SetVolumeMounts(deployment, existingMounts, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse())
		})

		It("should return error for non-existent container", func() {
			mounts := []corev1.VolumeMount{{Name: "vol1", MountPath: "/data"}}
			_, err := SetVolumeMounts(deployment, mounts, "non-existent")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container non-existent not found"))
		})

		It("should handle empty volume mounts", func() {
			changed, err := SetVolumeMounts(deployment, []corev1.VolumeMount{}, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(BeEmpty())
		})
	})

	Describe("SetDeploymentContainerEnvs", func() {
		It("should set environment variables when different", func() {
			newEnvs := []corev1.EnvVar{
				{Name: "ENV2", Value: "value2"},
				{Name: "ENV3", Value: "value3"},
			}
			changed, err := SetDeploymentContainerEnvs(deployment, newEnvs, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(Equal(newEnvs))
		})

		It("should not update when envs are the same", func() {
			existingEnvs := deployment.Spec.Template.Spec.Containers[0].Env
			changed, err := SetDeploymentContainerEnvs(deployment, existingEnvs, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse())
		})

		It("should return error for non-existent container", func() {
			envs := []corev1.EnvVar{{Name: "ENV1", Value: "value1"}}
			_, err := SetDeploymentContainerEnvs(deployment, envs, "non-existent")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container non-existent not found"))
		})

		It("should handle empty env vars", func() {
			changed, err := SetDeploymentContainerEnvs(deployment, []corev1.EnvVar{}, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(BeEmpty())
		})
	})

	Describe("SetDeploymentContainerResources", func() {
		It("should set resources when different", func() {
			newResources := &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
					corev1.ResourceCPU:    resource.MustParse("1000m"),
				},
			}
			changed, err := SetDeploymentContainerResources(deployment, newResources, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Containers[0].Resources).To(Equal(*newResources))
		})

		It("should not update when resources are the same", func() {
			existingResources := &deployment.Spec.Template.Spec.Containers[0].Resources
			changed, err := SetDeploymentContainerResources(deployment, existingResources, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse())
		})

		It("should return error for non-existent container", func() {
			resources := &corev1.ResourceRequirements{}
			_, err := SetDeploymentContainerResources(deployment, resources, "non-existent")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container non-existent not found"))
		})
	})

	Describe("SetDeploymentContainerVolumeMounts", func() {
		It("should set volume mounts when different", func() {
			newMounts := []corev1.VolumeMount{
				{Name: "vol2", MountPath: "/config"},
			}
			changed, err := SetDeploymentContainerVolumeMounts(deployment, "app-container", newMounts)

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeTrue())
			Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).To(Equal(newMounts))
		})

		It("should not update when volume mounts are the same", func() {
			existingMounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
			changed, err := SetDeploymentContainerVolumeMounts(deployment, "app-container", existingMounts)

			Expect(err).NotTo(HaveOccurred())
			Expect(changed).To(BeFalse())
		})

		It("should return error for non-existent container", func() {
			mounts := []corev1.VolumeMount{{Name: "vol1", MountPath: "/data"}}
			_, err := SetDeploymentContainerVolumeMounts(deployment, "non-existent", mounts)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container non-existent not found"))
		})
	})

	Describe("GetContainerIndex", func() {
		It("should return correct index for existing container", func() {
			index, err := GetContainerIndex(deployment, "app-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(index).To(Equal(0))
		})

		It("should return correct index for second container", func() {
			index, err := GetContainerIndex(deployment, "sidecar-container")

			Expect(err).NotTo(HaveOccurred())
			Expect(index).To(Equal(1))
		})

		It("should return error for non-existent container", func() {
			_, err := GetContainerIndex(deployment, "non-existent")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("container non-existent not found"))
			Expect(err.Error()).To(ContainSubstring("test-deployment"))
		})
	})

	Describe("SetDefaults_Deployment", func() {
		var deployment *appsv1.Deployment

		BeforeEach(func() {
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-deployment",
					Namespace: "test-namespace",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "app", Image: "myapp:v1"},
							},
						},
					},
				},
			}
		})

		It("should set default replicas to 1", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Replicas).NotTo(BeNil())
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should not override existing replicas", func() {
			replicas := int32(3)
			deployment.Spec.Replicas = &replicas

			SetDefaults_Deployment(deployment)

			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should set default strategy to RollingUpdate", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
		})

		It("should set default MaxUnavailable to 25%", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.RollingUpdate).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.String()).To(Equal("25%"))
		})

		It("should set default MaxSurge to 25%", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.RollingUpdate).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge.String()).To(Equal("25%"))
		})

		It("should set default RevisionHistoryLimit to 10", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.RevisionHistoryLimit).NotTo(BeNil())
			Expect(*deployment.Spec.RevisionHistoryLimit).To(Equal(int32(10)))
		})

		It("should set default ProgressDeadlineSeconds to 600", func() {
			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.ProgressDeadlineSeconds).NotTo(BeNil())
			Expect(*deployment.Spec.ProgressDeadlineSeconds).To(Equal(int32(600)))
		})

		It("should not override existing strategy settings", func() {
			maxUnavailable := intstr.FromInt(1)
			maxSurge := intstr.FromInt(2)
			deployment.Spec.Strategy = appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			}

			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(1))
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge.IntValue()).To(Equal(2))
		})

		It("should handle Recreate strategy type", func() {
			deployment.Spec.Strategy.Type = appsv1.RecreateDeploymentStrategyType

			SetDefaults_Deployment(deployment)

			Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
			Expect(deployment.Spec.Strategy.RollingUpdate).To(BeNil())
		})
	})
})
