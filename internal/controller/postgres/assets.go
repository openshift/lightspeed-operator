package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func GetPostgresCAConfigVolume() corev1.Volume {
	volumeDefaultMode := utils.VolumeDefaultMode
	return corev1.Volume{
		Name: utils.PostgresCAVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: utils.OLSCAConfigMap,
				},
				DefaultMode: &volumeDefaultMode,
			},
		},
	}
}

func GetPostgresCAVolumeMount(mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      utils.PostgresCAVolume,
		MountPath: mountPath,
		ReadOnly:  true,
	}
}

func GeneratePostgresDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	cacheReplicas := int32(1)
	revisionHistoryLimit := int32(1)

	// Get postgres secret name (can be customized via CR or use default)
	postgresSecretName := utils.PostgresSecretName
	if cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}

	passwordMap, err := utils.GetSecretContent(r, postgresSecretName, r.GetNamespace(), []string{utils.OLSComponentPasswordFileName}, &corev1.Secret{})
	if err != nil {
		return nil, fmt.Errorf("password is needed to start postgres deployment : %w", err)
	}
	postgresPassword := passwordMap[utils.OLSComponentPasswordFileName]
	if cr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers == "" {
		cr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers = utils.PostgresSharedBuffers
	}
	if cr.Spec.OLSConfig.ConversationCache.Postgres.MaxConnections == 0 {
		cr.Spec.OLSConfig.ConversationCache.Postgres.MaxConnections = utils.PostgresMaxConnections
	}

	// Initialize volumes and volume mounts slices
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	// TLS certs volume and mount (for secure postgres connection)
	defaultPermission := utils.VolumeRestrictedMode
	tlsCertsVolume := corev1.Volume{
		Name: "secret-" + utils.PostgresCertsSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  utils.PostgresCertsSecretName,
				DefaultMode: &defaultPermission,
			},
		},
	}
	postgresTLSVolumeMount := corev1.VolumeMount{
		Name:      "secret-" + utils.PostgresCertsSecretName,
		MountPath: utils.OLSAppCertsMountRoot,
		ReadOnly:  true,
	}
	volumes = append(volumes, tlsCertsVolume)
	volumeMounts = append(volumeMounts, postgresTLSVolumeMount)

	// Bootstrap script volume and mount (for creating postgres extensions)
	bootstrapVolume := corev1.Volume{
		Name: "secret-" + utils.PostgresBootstrapSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  utils.PostgresBootstrapSecretName,
				DefaultMode: &defaultPermission,
			},
		},
	}
	bootstrapVolumeMount := corev1.VolumeMount{
		Name:      "secret-" + utils.PostgresBootstrapSecretName,
		MountPath: utils.PostgresBootstrapVolumeMountPath,
		SubPath:   utils.PostgresExtensionScript,
		ReadOnly:  true,
	}
	volumes = append(volumes, bootstrapVolume)
	volumeMounts = append(volumeMounts, bootstrapVolumeMount)

	// Config volume and mount (postgres configuration file)
	volumeDefaultMode := utils.VolumeDefaultMode
	configVolume := corev1.Volume{
		Name: utils.PostgresConfigMap,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: utils.PostgresConfigMap},
				DefaultMode:          &volumeDefaultMode,
			},
		},
	}
	configVolumeMount := corev1.VolumeMount{
		Name:      utils.PostgresConfigMap,
		MountPath: utils.PostgresConfigVolumeMountPath,
		SubPath:   utils.PostgresConfig,
	}
	volumes = append(volumes, configVolume)
	volumeMounts = append(volumeMounts, configVolumeMount)

	// Data volume and mount (postgres data directory - PVC or emptyDir)
	dataVolume := corev1.Volume{
		Name: utils.PostgresDataVolume,
	}
	if cr.Spec.OLSConfig.Storage != nil {
		dataVolume.VolumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: utils.PostgresPVCName,
			},
		}
	} else {
		dataVolume.VolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}
	dataVolumeMount := corev1.VolumeMount{
		Name:      utils.PostgresDataVolume,
		MountPath: utils.PostgresDataVolumeMountPath,
	}
	volumes = append(volumes, dataVolume)
	volumeMounts = append(volumeMounts, dataVolumeMount)

	// Postgres CA volume and mount (for TLS certificate verification)
	volumes = append(volumes, GetPostgresCAConfigVolume())
	volumeMounts = append(volumeMounts, GetPostgresCAVolumeMount(path.Join(utils.OLSAppCertsMountRoot, utils.PostgresCAVolume)))

	// Var run volume and mount (writable directory for postgres runtime files)
	varRunVolume := corev1.Volume{
		Name: utils.PostgresVarRunVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	varRunVolumeMount := corev1.VolumeMount{
		Name:      utils.PostgresVarRunVolumeName,
		MountPath: utils.PostgresVarRunVolumeMountPath,
	}
	volumes = append(volumes, varRunVolume)
	volumeMounts = append(volumeMounts, varRunVolumeMount)

	// Tmp volume and mount (writable temporary directory)
	tmpVolume := corev1.Volume{
		Name: utils.TmpVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	tmpVolumeMount := corev1.VolumeMount{
		Name:      utils.TmpVolumeName,
		MountPath: utils.TmpVolumeMountPath,
	}
	volumes = append(volumes, tmpVolume)
	volumeMounts = append(volumeMounts, tmpVolumeMount)

	databaseResources := getDatabaseResources(cr)

	// Get ResourceVersions for tracking - these resources should already exist
	// If they don't exist, we'll get empty strings which is fine for initial creation
	configMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.PostgresConfigMap)
	secretResourceVersion, _ := utils.GetSecretResourceVersion(r, ctx, postgresSecretName)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
			Annotations: map[string]string{
				utils.PostgresConfigMapResourceVersionAnnotation: configMapResourceVersion,
				utils.PostgresSecretResourceVersionAnnotation:    secretResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &cacheReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: utils.GeneratePostgresSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: utils.GeneratePostgresSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            utils.PostgresDeploymentName,
							Image:           r.GetPostgresImage(),
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "server",
									ContainerPort: utils.PostgresServicePort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								ReadOnlyRootFilesystem:   &[]bool{true}[0],
							},
							VolumeMounts: volumeMounts,
							Resources:    *databaseResources,
							Env: []corev1.EnvVar{
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
									Value: postgresPassword,
								},
								{
									Name:  "POSTGRESQL_PASSWORD",
									Value: postgresPassword,
								},
								{
									Name:  "POSTGRESQL_SHARED_BUFFERS",
									Value: cr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers,
								},
								{
									Name:  "POSTGRESQL_MAX_CONNECTIONS",
									Value: strconv.Itoa(cr.Spec.OLSConfig.ConversationCache.Postgres.MaxConnections),
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}

	if cr.Spec.OLSConfig.DeploymentConfig.DatabaseContainer.Tolerations != nil {
		deployment.Spec.Template.Spec.Tolerations = cr.Spec.OLSConfig.DeploymentConfig.DatabaseContainer.Tolerations
	}
	if cr.Spec.OLSConfig.DeploymentConfig.DatabaseContainer.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = cr.Spec.OLSConfig.DeploymentConfig.DatabaseContainer.NodeSelector
	}
	if err := controllerutil.SetControllerReference(cr, &deployment, r.GetScheme()); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// updatePostgresDeployment updates the deployment based on CustomResource configuration.
func UpdatePostgresDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	// Get the actual secret name from the CR (can be customized via CR or use default)
	postgresSecretName := utils.PostgresSecretName
	if cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}

	// Step 1: Check if deployment spec has changed
	utils.SetDefaults_Deployment(desiredDeployment)
	changed := !utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec)

	// Step 2: Check ConfigMap and Secret ResourceVersions
	// Check if ConfigMap ResourceVersion has changed
	currentConfigMapVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.PostgresConfigMap)
	if err != nil {
		r.GetLogger().Info("failed to get ConfigMap ResourceVersion", "error", err)
		changed = true
	} else {
		storedConfigMapVersion := existingDeployment.Annotations[utils.PostgresConfigMapResourceVersionAnnotation]
		if storedConfigMapVersion != currentConfigMapVersion {
			changed = true
		}
	}

	// Check if Secret ResourceVersion has changed (using the actual secret name from CR)
	currentSecretVersion, err := utils.GetSecretResourceVersion(r, ctx, postgresSecretName)
	if err != nil {
		r.GetLogger().Info("failed to get Secret ResourceVersion", "error", err)
		changed = true
	} else {
		storedSecretVersion := existingDeployment.Annotations[utils.PostgresSecretResourceVersionAnnotation]
		if storedSecretVersion != currentSecretVersion {
			changed = true
		}
	}

	// If nothing changed, skip update
	if !changed {
		return nil
	}

	// Apply changes - always update spec and annotations since something changed
	existingDeployment.Spec = desiredDeployment.Spec
	existingDeployment.Annotations[utils.PostgresConfigMapResourceVersionAnnotation] = desiredDeployment.Annotations[utils.PostgresConfigMapResourceVersionAnnotation]
	existingDeployment.Annotations[utils.PostgresSecretResourceVersionAnnotation] = desiredDeployment.Annotations[utils.PostgresSecretResourceVersionAnnotation]

	r.GetLogger().Info("updating OLS postgres deployment", "name", existingDeployment.Name)

	if err := RestartPostgres(r, ctx, existingDeployment); err != nil {
		return err
	}

	return nil
}

func GeneratePostgresService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresServiceName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.PostgresCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       utils.PostgresServicePort,
					Protocol:   corev1.ProtocolTCP,
					Name:       "server",
					TargetPort: intstr.Parse("server"),
				},
			},
			Selector: utils.GeneratePostgresSelectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GeneratePostgresSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	// Get postgres secret name (can be customized via CR or use default)
	postgresSecretName := utils.PostgresSecretName
	if cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}

	randomPassword := make([]byte, 12)
	_, err := rand.Read(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("error generating random password: %w", err)
	}
	// Encode the password to base64
	encodedPassword := base64.StdEncoding.EncodeToString(randomPassword)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresSecretName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		Data: map[string][]byte{
			utils.PostgresSecretKeyName: []byte(encodedPassword),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return &secret, nil
}

func GeneratePostgresBootstrapSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresBootstrapSecretName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		StringData: map[string]string{
			utils.PostgresExtensionScript: string(utils.PostgresBootStrapScriptContent),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return &secret, nil
}

func GeneratePostgresConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresConfigMap,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		Data: map[string]string{
			utils.PostgresConfig: utils.PostgresConfigMapContent,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &configMap, r.GetScheme()); err != nil {
		return nil, err
	}

	return &configMap, nil
}

func GeneratePostgresNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
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
				},
			},
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GeneratePostgresSelectorLabels(),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}
	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, err
	}
	return &np, nil
}

func storageDefaults(r reconciler.Reconciler, s *olsv1alpha1.Storage) error {
	if s.Size.IsZero() {
		s.Size = resource.MustParse(utils.PostgresDefaultPVCSize)
	}
	if s.Class == "" {
		var scList storagev1.StorageClassList
		ctx := context.Background()
		if err := r.List(ctx, &scList); err == nil {
			for _, sc := range scList.Items {
				if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
					s.Class = sc.Name
				}
			}
		}
		if s.Class == "" {
			return fmt.Errorf("no storage class specified and no default storage class configured")
		}
	}
	return nil
}

func GeneratePostgresPVC(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.PersistentVolumeClaim, error) {

	storage := cr.Spec.OLSConfig.Storage
	if err := storageDefaults(r, storage); err != nil {
		return nil, err
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresPVCName,
			Namespace: r.GetNamespace(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode("ReadWriteOnce"),
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storage.Size,
				},
			},
			StorageClassName: &storage.Class,
		},
	}

	if err := controllerutil.SetControllerReference(cr, pvc, r.GetScheme()); err != nil {
		return nil, err
	}
	return pvc, nil
}

func getDatabaseResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.DatabaseContainer.Resources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("30m"),
				corev1.ResourceMemory: resource.MustParse("300Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		},
	)
}
