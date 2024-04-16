package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func generatePostgresSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "postgres-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-postgres",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

func getPostgresCAConfigVolume() corev1.Volume {
	return corev1.Volume{
		Name: PostgresCAVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OLSCAConfigMap,
				},
			},
		},
	}
}

func getPostgresCAVolumeMount(mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      PostgresCAVolume,
		MountPath: mountPath,
		ReadOnly:  true,
	}
}

func (r *OLSConfigReconciler) generatePostgresDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	cacheReplicas := int32(1)
	revisionHistoryLimit := int32(1)
	postgresSecretName := PostgresSecretName
	if cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}
	postgresPassword, err := getSecretContent(r.Client, postgresSecretName, r.Options.Namespace, OLSComponentPasswordFileName, &corev1.Secret{})
	if err != nil {
		return nil, fmt.Errorf("Password is a must to start postgres deployment : %w", err)
	}
	postgresSharedBuffers := intstr.FromString(PostgresSharedBuffers)
	postgresConfig := cr.Spec.OLSConfig.ConversationCache.Postgres
	if postgresConfig.SharedBuffers == nil || postgresConfig.SharedBuffers.String() == "" {
		cr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers = &postgresSharedBuffers
	}
	if postgresConfig.MaxConnections == 0 {
		cr.Spec.OLSConfig.ConversationCache.Postgres.MaxConnections = PostgresMaxConnections
	}
	defaultPermission := int32(0600)
	tlsCertsVolume := corev1.Volume{
		Name: "secret-" + PostgresCertsSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  PostgresCertsSecretName,
				DefaultMode: &defaultPermission,
			},
		},
	}
	bootstrapVolume := corev1.Volume{
		Name: "secret-" + PostgresBootstrapSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: PostgresBootstrapSecretName,
			},
		},
	}
	configVolume := corev1.Volume{
		Name: PostgresConfigMap,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: PostgresConfigMap},
			},
		},
	}
	dataVolume := corev1.Volume{
		Name: PostgresDataVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volumes := []corev1.Volume{tlsCertsVolume, bootstrapVolume, configVolume, dataVolume, getPostgresCAConfigVolume()}
	postgresTLSVolumeMount := corev1.VolumeMount{
		Name:      "secret-" + PostgresCertsSecretName,
		MountPath: OLSAppCertsMountRoot,
		ReadOnly:  true,
	}
	bootstrapVolumeMount := corev1.VolumeMount{
		Name:      "secret-" + PostgresBootstrapSecretName,
		MountPath: PostgresBootstrapVolumeMount,
		SubPath:   PostgresExtensionScript,
		ReadOnly:  true,
	}
	configVolumeMount := corev1.VolumeMount{
		Name:      PostgresConfigMap,
		MountPath: PostgresConfigVolumeMount,
		SubPath:   PostgresConfig,
	}
	dataVolumeMount := corev1.VolumeMount{
		Name:      PostgresDataVolume,
		MountPath: PostgresDataVolumeMount,
	}
	volumeMounts := []corev1.VolumeMount{postgresTLSVolumeMount, bootstrapVolumeMount, configVolumeMount, dataVolumeMount, getPostgresCAVolumeMount(path.Join(OLSAppCertsMountRoot, PostgresCAVolume))}
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresDeploymentName,
			Namespace: r.Options.Namespace,
			Labels:    generatePostgresSelectorLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &cacheReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: generatePostgresSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: generatePostgresSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            PostgresDeploymentName,
							Image:           r.Options.LightspeedServicePostgresImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									Name:          "server",
									ContainerPort: PostgresServicePort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: volumeMounts,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRESQL_USER",
									Value: PostgresDefaultUser,
								},
								{
									Name:  "POSTGRESQL_DATABASE",
									Value: PostgresDefaultDbName,
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
									Value: cr.Spec.OLSConfig.ConversationCache.Postgres.SharedBuffers.StrVal,
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

	if err := controllerutil.SetControllerReference(cr, &deployment, r.Scheme); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// updatePostgresDeployment updates the deployment based on CustomResource configuration.
func (r *OLSConfigReconciler) updatePostgresDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	changed := false

	// Validate deployment annotations.
	if existingDeployment.Annotations == nil ||
		existingDeployment.Annotations[PostgresConfigHashKey] != r.stateCache[PostgresConfigHashStateCacheKey] ||
		existingDeployment.Annotations[PostgresSecretHashKey] != r.stateCache[PostgresSecretHashStateCacheKey] {
		updateDeploymentAnnotations(existingDeployment, map[string]string{
			PostgresConfigHashKey: r.stateCache[PostgresConfigHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		// update the deployment template annotation triggers the rolling update
		updateDeploymentTemplateAnnotations(existingDeployment, map[string]string{
			PostgresConfigHashKey: r.stateCache[PostgresConfigHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})

		if _, err := setDeploymentContainerEnvs(existingDeployment, desiredDeployment.Spec.Template.Spec.Containers[0].Env, PostgresDeploymentName); err != nil {
			return err
		}

		changed = true
	}

	if changed {
		r.logger.Info("updating OLS postgres deployment", "name", existingDeployment.Name)
		if err := r.Update(ctx, existingDeployment); err != nil {
			return err
		}
	} else {
		r.logger.Info("OLS postgres deployment reconciliation skipped", "deployment", existingDeployment.Name, "olsconfig hash", existingDeployment.Annotations[PostgresConfigHashKey])
	}

	return nil
}

func (r *OLSConfigReconciler) generatePostgresService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresServiceName,
			Namespace: r.Options.Namespace,
			Labels:    generatePostgresSelectorLabels(),
			Annotations: map[string]string{
				ServingCertSecretAnnotationKey: PostgresCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       PostgresServicePort,
					Protocol:   corev1.ProtocolTCP,
					Name:       "server",
					TargetPort: intstr.Parse("server"),
				},
			},
			Selector: generatePostgresSelectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.Scheme); err != nil {
		return nil, err
	}

	return &service, nil
}

func (r *OLSConfigReconciler) generatePostgresSecret(cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	postgresSecretName := PostgresSecretName
	if cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}
	randomPassword := make([]byte, 12)
	_, err := rand.Read(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("Error generating random password: %w", err)
	}
	// Encode the password to base64
	encodedPassword := base64.StdEncoding.EncodeToString(randomPassword)
	passwordHash, err := hashBytes([]byte(encodedPassword))
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS postgres password hash %w", err)
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      postgresSecretName,
			Namespace: r.Options.Namespace,
			Labels:    generatePostgresSelectorLabels(),
			Annotations: map[string]string{
				PostgresSecretHashKey: passwordHash,
			},
		},
		Data: map[string][]byte{
			PostgresSecretKeyName: []byte(encodedPassword),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.Scheme); err != nil {
		return nil, err
	}

	return &secret, nil
}

func (r *OLSConfigReconciler) generatePgBootstrapSecret(cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresBootstrapSecretName,
			Namespace: r.Options.Namespace,
			Labels:    generatePostgresSelectorLabels(),
		},
		StringData: map[string]string{
			PostgresExtensionScript: string(PostgresBootStrapScriptContent),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.Scheme); err != nil {
		return nil, err
	}

	return &secret, nil
}

func (r *OLSConfigReconciler) generatePgConfigMap(cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PostgresConfigMap,
			Namespace: r.Options.Namespace,
			Labels:    generatePostgresSelectorLabels(),
		},
		Data: map[string]string{
			PostgresConfig: PostgresConfigMapContent,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &configMap, r.Scheme); err != nil {
		return nil, err
	}

	return &configMap, nil
}
