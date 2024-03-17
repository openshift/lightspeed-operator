package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func generateRedisSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "redis-server",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-service-redis",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

func getRedisCAConfigVolume() corev1.Volume {
	return corev1.Volume{
		Name: RedisCAVolume,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: RedisCAConfigMap,
				},
			},
		},
	}
}

func getRedisCAVolumeMount(mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      RedisCAVolume,
		MountPath: mountPath,
		ReadOnly:  true,
	}
}

func (r *OLSConfigReconciler) generateRedisDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	cacheReplicas := int32(1)
	revisionHistoryLimit := int32(0)
	redisPassword, err := getSecretContent(r.Client, cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret, cr.Namespace, OLSComponentPasswordFileName)
	if err != nil {
		return nil, fmt.Errorf("Password is a must to start redis deployment : %w", err)
	}
	tlsCertsVolume := corev1.Volume{
		Name: "secret-" + RedisCertsSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: RedisCertsSecretName,
			},
		},
	}
	volumes := []corev1.Volume{tlsCertsVolume, getRedisCAConfigVolume()}
	redisTLSVolumeMount := corev1.VolumeMount{
		Name:      "secret-" + RedisCertsSecretName,
		MountPath: OLSAppCertsMountRoot,
		ReadOnly:  true,
	}
	volumeMounts := []corev1.VolumeMount{redisTLSVolumeMount, getRedisCAVolumeMount(path.Join(OLSAppCertsMountRoot, RedisCAVolume))}
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RedisDeploymentName,
			Namespace: cr.Namespace,
			Labels:    generateRedisSelectorLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &cacheReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: generateRedisSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: generateRedisSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "lightspeed-redis-server",
							Image:           r.Options.LightspeedServiceRedisImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: RedisServicePort,
									Name:          "server",
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
							Command: []string{"redis-server",
								"--port", "0",
								"--tls-port", "6379",
								"--tls-cert-file", path.Join(OLSAppCertsMountRoot, "tls.crt"),
								"--tls-key-file", path.Join(OLSAppCertsMountRoot, "tls.key"),
								"--tls-ca-cert-file", path.Join(OLSAppCertsMountRoot, RedisCAVolume, "service-ca.crt"),
								"--tls-auth-clients", "optional",
								"--protected-mode", "no",
								"--requirepass", redisPassword},
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

// updateRedisDeployment updates the deployment based on CustomResource configuration.
func (r *OLSConfigReconciler) updateRedisDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	changed := false

	// Validate deployment annotations.
	if existingDeployment.Annotations == nil ||
		existingDeployment.Annotations[RedisConfigHashKey] != r.stateCache[RedisConfigHashStateCacheKey] {
		updateDeploymentAnnotations(existingDeployment, map[string]string{
			RedisConfigHashKey: r.stateCache[RedisConfigHashStateCacheKey],
		})
		// update the deployment template annotation triggers the rolling update
		updateDeploymentTemplateAnnotations(existingDeployment, map[string]string{
			RedisConfigHashKey: r.stateCache[RedisConfigHashStateCacheKey],
		})

		changed = true
	}

	if commandChanged, err := setCommand(existingDeployment, desiredDeployment.Spec.Template.Spec.Containers[0].Command, RedisDeploymentName); err != nil {
		return err
	} else if commandChanged {
		changed = true
	}

	if changed {
		r.logger.Info("updating OLS redis deployment", "name", existingDeployment.Name)
		if err := r.Update(ctx, existingDeployment); err != nil {
			return err
		}
	} else {
		r.logger.Info("OLS redis deployment reconciliation skipped", "deployment", existingDeployment.Name, "olsconfig hash", existingDeployment.Annotations[RedisConfigHashKey])
	}

	return nil
}

func (r *OLSConfigReconciler) generateRedisService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RedisServiceName,
			Namespace: cr.Namespace,
			Labels:    generateRedisSelectorLabels(),
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": RedisCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       RedisServicePort,
					Protocol:   corev1.ProtocolTCP,
					Name:       "server",
					TargetPort: intstr.Parse("server"),
				},
			},
			Selector: generateRedisSelectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.Scheme); err != nil {
		return nil, err
	}

	return &service, nil
}

func (r *OLSConfigReconciler) generateRedisSecret(cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	randomPassword := make([]byte, 12)
	_, err := rand.Read(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("Error generating random password: %w", err)
	}
	// Encode the password to base64
	encodedPassword := base64.StdEncoding.EncodeToString(randomPassword)
	passwordHash, err := hashBytes([]byte(encodedPassword))
	if err != nil {
		return nil, fmt.Errorf("failed to generate OLS redis password hash %w", err)
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecret,
			Namespace: cr.Namespace,
			Labels:    generateRedisSelectorLabels(),
			Annotations: map[string]string{
				RedisSecretHashKey: passwordHash,
			},
		},
		Data: map[string][]byte{
			RedisSecretKeyName: []byte(encodedPassword),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.Scheme); err != nil {
		return nil, err
	}

	return &secret, nil
}
