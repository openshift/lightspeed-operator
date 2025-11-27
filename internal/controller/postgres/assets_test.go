package postgres

import (
	"path"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

var _ = Describe("App postgres server assets", func() {

	var testCr *olsv1alpha1.OLSConfig

	validatePostgresDeployment := func(dep *appsv1.Deployment, password string, with_pvc bool) {
		replicas := int32(1)
		revisionHistoryLimit := int32(1)
		defaultPermission := utils.VolumeRestrictedMode
		volumeDefaultMode := utils.VolumeDefaultMode
		Expect(dep.Name).To(Equal(utils.PostgresDeploymentName))
		Expect(dep.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(utils.PostgresServerImageDefault))
		Expect(dep.Spec.Template.Spec.Containers[0].Name).To(Equal(utils.PostgresContainerName))
		Expect(dep.Spec.Template.Spec.Containers[0].ImagePullPolicy).To(Equal(corev1.PullAlways))
		Expect(dep.Spec.Template.Spec.Containers[0].Ports).To(Equal([]corev1.ContainerPort{
			{
				ContainerPort: utils.PostgresServicePort,
				Name:          "server",
				Protocol:      corev1.ProtocolTCP,
			},
		}))
		Expect(dep.Spec.Template.Spec.Containers[0].Resources).To(Equal(corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("30m"),
				corev1.ResourceMemory: resource.MustParse("300Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		}))
		Expect(dep.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
			{
				Name:  "POSTGRESQL_USER",
				Value: utils.PostgresDefaultUser,
			},
			{
				Name:  "POSTGRESQL_DATABASE",
				Value: utils.PostgresDefaultDbName,
			},
			{
				Name:  "POSTGRESQL_ADMIN_PASSWORD",
				Value: password,
			},
			{
				Name:  "POSTGRESQL_PASSWORD",
				Value: password,
			},
			{
				Name:  "POSTGRESQL_SHARED_BUFFERS",
				Value: utils.PostgresSharedBuffers,
			},
			{
				Name:  "POSTGRESQL_MAX_CONNECTIONS",
				Value: strconv.Itoa(utils.PostgresMaxConnections),
			},
		}))
		Expect(dep.Spec.Selector.MatchLabels).To(Equal(utils.GeneratePostgresSelectorLabels()))
		Expect(dep.Spec.RevisionHistoryLimit).To(Equal(&revisionHistoryLimit))
		Expect(dep.Spec.Replicas).To(Equal(&replicas))
		Expect(dep.Spec.Template.Spec.Containers[0].VolumeMounts).To(Equal([]corev1.VolumeMount{
			{
				Name:      "secret-" + utils.PostgresCertsSecretName,
				MountPath: utils.OLSAppCertsMountRoot,
				ReadOnly:  true,
			},
			{
				Name:      "secret-" + utils.PostgresBootstrapSecretName,
				MountPath: utils.PostgresBootstrapVolumeMountPath,
				SubPath:   utils.PostgresExtensionScript,
				ReadOnly:  true,
			},
			{
				Name:      utils.PostgresConfigMap,
				MountPath: utils.PostgresConfigVolumeMountPath,
				SubPath:   utils.PostgresConfig,
			},
			{
				Name:      utils.PostgresDataVolume,
				MountPath: utils.PostgresDataVolumeMountPath,
			},
			{
				Name:      utils.PostgresCAVolume,
				MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.PostgresCAVolume),
				ReadOnly:  true,
			},
			{
				Name:      utils.PostgresVarRunVolumeName,
				MountPath: utils.PostgresVarRunVolumeMountPath,
			},
			{
				Name:      utils.TmpVolumeName,
				MountPath: utils.TmpVolumeMountPath,
			},
		}))
		expectedVolumes := []corev1.Volume{
			{
				Name: "secret-" + utils.PostgresCertsSecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  utils.PostgresCertsSecretName,
						DefaultMode: &defaultPermission,
					},
				},
			},
			{
				Name: "secret-" + utils.PostgresBootstrapSecretName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  utils.PostgresBootstrapSecretName,
						DefaultMode: &defaultPermission,
					},
				},
			},
			{
				Name: utils.PostgresConfigMap,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: utils.PostgresConfigMap},
						DefaultMode:          &volumeDefaultMode,
					},
				},
			},
		}
		if with_pvc {
			expectedVolumes = append(expectedVolumes, corev1.Volume{
				Name: utils.PostgresDataVolume,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: utils.PostgresPVCName,
					},
				},
			})
		} else {
			expectedVolumes = append(expectedVolumes, corev1.Volume{
				Name: utils.PostgresDataVolume,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
		}
		expectedVolumes = append(expectedVolumes,
			corev1.Volume{
				Name: utils.PostgresCAVolume,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: utils.OLSCAConfigMap},
						DefaultMode:          &volumeDefaultMode,
					},
				},
			},
			corev1.Volume{
				Name: utils.PostgresVarRunVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			corev1.Volume{
				Name: utils.TmpVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		)
		Expect(dep.Spec.Template.Spec.Volumes).To(Equal(expectedVolumes))
	}

	validatePostgresService := func(service *corev1.Service, err error) {
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Name).To(Equal(utils.PostgresServiceName))
		Expect(service.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(service.Labels).To(Equal(utils.GeneratePostgresSelectorLabels()))
		Expect(service.Annotations).To(Equal(map[string]string{
			"service.beta.openshift.io/serving-cert-secret-name": utils.PostgresCertsSecretName,
		}))
		Expect(service.Spec.Selector).To(Equal(utils.GeneratePostgresSelectorLabels()))
		Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		Expect(service.Spec.Ports).To(Equal([]corev1.ServicePort{
			{
				Name:       "server",
				Port:       utils.PostgresServicePort,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.Parse("server"),
			},
		}))
	}

	validatePostgresConfigMap := func(configMap *corev1.ConfigMap) {
		Expect(configMap.Namespace).To(Equal(testCr.Namespace))
		Expect(configMap.Labels).To(Equal(utils.GeneratePostgresSelectorLabels()))
		Expect(configMap.Data).To(HaveKey(utils.PostgresConfig))
	}

	validatePostgresSecret := func(secret *corev1.Secret) {
		Expect(secret.Namespace).To(Equal(testCr.Namespace))
		Expect(secret.Labels).To(Equal(utils.GeneratePostgresSelectorLabels()))
		Expect(secret.Data).To(HaveKey(utils.PostgresSecretKeyName))
	}

	validatePostgresBootstrapSecret := func(secret *corev1.Secret) {
		Expect(secret.Namespace).To(Equal(testCr.Namespace))
		Expect(secret.Labels).To(Equal(utils.GeneratePostgresSelectorLabels()))
		Expect(secret.StringData).To(HaveKey(utils.PostgresExtensionScript))
	}

	validatePostgresNetworkPolicy := func(networkPolicy *networkingv1.NetworkPolicy) {
		Expect(networkPolicy.Name).To(Equal(utils.PostgresNetworkPolicyName))
		Expect(networkPolicy.Namespace).To(Equal(utils.OLSNamespaceDefault))
		Expect(networkPolicy.Spec.PolicyTypes).To(Equal([]networkingv1.PolicyType{networkingv1.PolicyTypeIngress}))
		Expect(networkPolicy.Spec.Ingress).To(HaveLen(1))
		Expect(networkPolicy.Spec.Ingress).To(ConsistOf(networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: utils.GenerateAppServerSelectorLabels(),
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
					Port:     &[]intstr.IntOrString{intstr.FromInt(utils.PostgresServicePort)}[0],
				},
			},
		}))
		Expect(networkPolicy.Spec.PodSelector.MatchLabels).To(Equal(utils.GeneratePostgresSelectorLabels()))
	}

	createAndValidatePostgresDeployment := func(with_pvc bool) {
		if with_pvc {
			testCr.Spec.OLSConfig.Storage = &olsv1alpha1.Storage{}
		}
		testCr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret-1"
		secret, _ := GeneratePostgresSecret(testReconcilerInstance, testCr)
		secret.SetOwnerReferences([]metav1.OwnerReference{
			{
				Kind:       "Secret",
				APIVersion: "v1",
				UID:        "ownerUID",
				Name:       "dummy-secret-1",
			},
		})
		secretCreationErr := testReconcilerInstance.Create(ctx, secret)
		Expect(secretCreationErr).NotTo(HaveOccurred())
		passwordMap, _ := utils.GetSecretContent(testReconcilerInstance, secret.Name, testCr.Namespace, []string{utils.OLSComponentPasswordFileName}, secret)
		password := passwordMap[utils.OLSComponentPasswordFileName]
		deployment, err := GeneratePostgresDeployment(testReconcilerInstance, ctx, testCr)
		Expect(err).NotTo(HaveOccurred())
		validatePostgresDeployment(deployment, password, with_pvc)
		secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
		Expect(secretDeletionErr).NotTo(HaveOccurred())
	}

	Context("complete custom resource", func() {
		BeforeEach(func() {
			testCr = utils.GetOLSConfigWithCacheCR()
		})

		It("should generate the OLS postgres deployment", func() {
			createAndValidatePostgresDeployment(false)
		})

		It("should generate the OLS postgres deployment with a PVC data volume", func() {
			createAndValidatePostgresDeployment(true)
		})

		It("should customize container settings in the OLS postgres deployment", func() {
			resources := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("500Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("3Gi"),
				},
			}
			tolerations := []corev1.Toleration{
				{
					Key:      "test-key",
					Operator: corev1.TolerationOpEqual,
					Value:    "test-value",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			}
			nodeSelector := map[string]string{
				"test-node-selector-key": "test-node-selector-value",
			}
			testCr.Spec.OLSConfig.DeploymentConfig.DatabaseContainer = olsv1alpha1.DatabaseContainerConfig{
				Resources:    resources,
				Tolerations:  tolerations,
				NodeSelector: nodeSelector,
			}

			testCr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret-1"
			secret, _ := GeneratePostgresSecret(testReconcilerInstance, testCr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-1",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())

			deployment, err := GeneratePostgresDeployment(testReconcilerInstance, ctx, testCr)
			Expect(err).NotTo(HaveOccurred())

			Expect(deployment.Spec.Template.Spec.Containers[0].Resources).To(Equal(*resources))
			Expect(deployment.Spec.Template.Spec.Tolerations).To(Equal(tolerations))
			Expect(deployment.Spec.Template.Spec.NodeSelector).To(Equal(nodeSelector))
		})

		It("should work when no update in the OLS postgres deployment", func() {
			testCr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret-2"
			secret, _ := GeneratePostgresSecret(testReconcilerInstance, testCr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-2",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			deployment, err := GeneratePostgresDeployment(testReconcilerInstance, ctx, testCr)
			Expect(err).NotTo(HaveOccurred())
			deployment.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
					UID:        "ownerUID",
					Name:       "lightspeed-postgres-server-1",
				},
			})
			deployment.ObjectMeta.Name = "lightspeed-postgres-server-1"
			deploymentCreationErr := testReconcilerInstance.Create(ctx, deployment)
			Expect(deploymentCreationErr).NotTo(HaveOccurred())
			updateErr := UpdatePostgresDeployment(testReconcilerInstance, ctx, testCr, deployment, deployment)
			Expect(updateErr).NotTo(HaveOccurred())
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			deploymentDeletionErr := testReconcilerInstance.Delete(ctx, deployment)
			Expect(deploymentDeletionErr).NotTo(HaveOccurred())
		})

		It("should work when there is an update in the OLS postgres deployment", func() {
			testCr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret-3"
			secret, _ := GeneratePostgresSecret(testReconcilerInstance, testCr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-3",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			deployment, err := GeneratePostgresDeployment(testReconcilerInstance, ctx, testCr)
			Expect(err).NotTo(HaveOccurred())
			deployment.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Deployment",
					APIVersion: "apps/v1",
					UID:        "ownerUID",
					Name:       "lightspeed-postgres-server-2",
				},
			})
			deployment.ObjectMeta.Name = "lightspeed-postgres-server-2"
			deploymentCreationErr := testReconcilerInstance.Create(ctx, deployment)
			Expect(deploymentCreationErr).NotTo(HaveOccurred())
			deploymentClone := deployment.DeepCopy()
			deploymentClone.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
				{
					Name:  "DUMMY_UPDATE",
					Value: utils.PostgresDefaultUser,
				},
			}
			updateErr := UpdatePostgresDeployment(testReconcilerInstance, ctx, testCr, deployment, deploymentClone)
			Expect(updateErr).NotTo(HaveOccurred())
			Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(Equal([]corev1.EnvVar{
				{
					Name:  "DUMMY_UPDATE",
					Value: utils.PostgresDefaultUser,
				},
			}))
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
			deploymentDeletionErr := testReconcilerInstance.Delete(ctx, deployment)
			Expect(deploymentDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate the OLS postgres service", func() {
			validatePostgresService(GeneratePostgresService(testReconcilerInstance, testCr))
		})

		It("should generate the OLS postgres configmap", func() {
			configMap, err := GeneratePostgresConfigMap(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			Expect(configMap.Name).To(Equal(utils.PostgresConfigMap))
			validatePostgresConfigMap(configMap)
		})

		It("should generate the OLS postgres secret", func() {
			secret, err := GeneratePostgresSecret(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal(utils.PostgresSecretName))
			validatePostgresSecret(secret)
		})

		It("should generate the OLS postgres bootstrap secret", func() {
			secret, err := GeneratePostgresBootstrapSecret(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal(utils.PostgresBootstrapSecretName))
			validatePostgresBootstrapSecret(secret)
		})

		It("should generate the OLS postgres network policy", func() {
			networkPolicy, err := GeneratePostgresNetworkPolicy(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			validatePostgresNetworkPolicy(networkPolicy)
		})
	})

	Context("empty custom resource", func() {
		BeforeEach(func() {
			testCr = utils.GetNoCacheCR()
		})

		It("should generate the OLS postgres deployment", func() {
			testCr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = "dummy-secret-4"
			testCr.Spec.OLSConfig.ConversationCache.Postgres.User = utils.PostgresDefaultUser
			testCr.Spec.OLSConfig.ConversationCache.Postgres.DbName = utils.PostgresDefaultDbName
			testCr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers = utils.PostgresSharedBuffers
			testCr.Spec.OLSConfig.ConversationCache.Postgres.MaxConnections = utils.PostgresMaxConnections
			secret, _ := GeneratePostgresSecret(testReconcilerInstance, testCr)
			secret.SetOwnerReferences([]metav1.OwnerReference{
				{
					Kind:       "Secret",
					APIVersion: "v1",
					UID:        "ownerUID",
					Name:       "dummy-secret-4",
				},
			})
			secretCreationErr := testReconcilerInstance.Create(ctx, secret)
			Expect(secretCreationErr).NotTo(HaveOccurred())
			passwordMap, _ := utils.GetSecretContent(testReconcilerInstance, secret.Name, testCr.Namespace, []string{utils.OLSComponentPasswordFileName}, secret)
			password := passwordMap[utils.OLSComponentPasswordFileName]
			deployment, err := GeneratePostgresDeployment(testReconcilerInstance, ctx, testCr)
			Expect(err).NotTo(HaveOccurred())
			validatePostgresDeployment(deployment, password, false)
			secretDeletionErr := testReconcilerInstance.Delete(ctx, secret)
			Expect(secretDeletionErr).NotTo(HaveOccurred())
		})

		It("should generate the OLS postgres service", func() {
			validatePostgresService(GeneratePostgresService(testReconcilerInstance, testCr))
		})

		It("should generate the OLS postgres configmap", func() {
			configMap, err := GeneratePostgresConfigMap(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			Expect(configMap.Name).To(Equal(utils.PostgresConfigMap))
			validatePostgresConfigMap(configMap)
		})

		It("should generate the OLS postgres bootstrap secret", func() {
			secret, err := GeneratePostgresBootstrapSecret(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal(utils.PostgresBootstrapSecretName))
			validatePostgresBootstrapSecret(secret)
		})

		It("should generate the OLS postgres secret", func() {
			testCr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret = utils.PostgresSecretName
			secret, err := GeneratePostgresSecret(testReconcilerInstance, testCr)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.Name).To(Equal(utils.PostgresSecretName))
			validatePostgresSecret(secret)
		})
	})

})
