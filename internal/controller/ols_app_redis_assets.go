package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var DeploymentSelectorLabels = map[string]string{
	"app.kubernetes.io/component":  "redis-server",
	"app.kubernetes.io/managed-by": "lightspeed-operator",
	"app.kubernetes.io/name":       "lightspeed-service-redis",
	"app.kubernetes.io/part-of":    "openshift-lightspeed",
}

func (r *OLSConfigReconciler) generateRedisDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	cacheReplicas := int32(1)
	revisionHistoryLimit := int32(0)
	redisPassword, err := getSecretContent(r.Client, cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecretRef.Name, cr.Namespace, OLSPasswordFileName)
	if err != nil {
		return nil, fmt.Errorf("Password is a must to start redis deployment : %w", err)
	}
	volumes := []corev1.Volume{}
	tlsCertsVolume := corev1.Volume{
		Name: "secret-" + OLSAppRedisCertsSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: OLSAppRedisCertsSecretName,
			},
		},
	}
	redisCAConfigVolume := corev1.Volume{
		Name: OLSRedisCAVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OLSRedisCACmName,
				},
			},
		},
	}
	volumes = append(volumes, tlsCertsVolume, redisCAConfigVolume)
	volumeMounts := []corev1.VolumeMount{}
	redisTLSVolumeMount := corev1.VolumeMount{
		Name:      "secret-" + OLSAppRedisCertsSecretName,
		MountPath: OLSAppCertsMountRoot,
		ReadOnly:  true,
	}
	redisCAVolumeMount := corev1.VolumeMount{
		Name:      OLSRedisCAVolumeName,
		MountPath: path.Join(OLSAppCertsMountRoot, OLSRedisCAVolumeName),
		ReadOnly:  true,
	}
	volumeMounts = append(volumeMounts, redisTLSVolumeMount, redisCAVolumeMount)
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppRedisDeploymentName,
			Namespace: cr.Namespace,
			Labels:    DeploymentSelectorLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &cacheReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: DeploymentSelectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: DeploymentSelectorLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "lightspeed-redis-server",
							Image:           r.Options.LightspeedServiceRedisImage,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: OLSAppRedisServicePort,
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
								"--tls-ca-cert-file", path.Join(OLSAppCertsMountRoot, OLSRedisCAVolumeName, "service-ca.crt"),
								"--tls-auth-clients", "optional",
								"--protected-mode", "no",
								"--requirepass", redisPassword,
								"--loglevel", "verbose"},
						},
					},
					Volumes: volumes,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}
	if err = controllerutil.SetControllerReference(cr, &deployment, r.Scheme); err != nil {
		return nil, err
	}

	return &deployment, nil
}

// updateRedisDeployment updates the deployment based on CustomResource configuration.
func (r *OLSConfigReconciler) updateRedisDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	changed := false

	// Validate deployment annotations.
	if existingDeployment.Annotations == nil ||
		existingDeployment.Annotations[OLSConfigHashKey] != r.stateCache[OLSConfigHashStateCacheKey] {
		updateDeploymentAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
		})
		// update the deployment template annotation triggers the rolling update
		updateDeploymentTemplateAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey: r.stateCache[OLSConfigHashStateCacheKey],
		})

		changed = true
	}

	// Validate volume mounts for a specific container in deployment.
	if commandChanged, err := setCommand(existingDeployment, desiredDeployment.Spec.Template.Spec.Containers[0].Command, "lightspeed-redis-server"); err != nil {
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
		r.logger.Info("OLS redis deployment reconciliation skipped", "deployment", existingDeployment.Name, "olsconfig hash", existingDeployment.Annotations[OLSConfigHashKey])
	}

	return nil
}

func (r *OLSConfigReconciler) generateRedisService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	internalTrafficPolicy := corev1.ServiceInternalTrafficPolicyCluster
	ipFamilies := []corev1.IPFamily{corev1.IPv4Protocol}
	ipFamilyPolicy := corev1.IPFamilyPolicySingleStack
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppRedisServiceName,
			Namespace: cr.Namespace,
			Labels:    DeploymentSelectorLabels,
			Annotations: map[string]string{
				"service.beta.openshift.io/serving-cert-secret-name": OLSAppRedisCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			InternalTrafficPolicy: &internalTrafficPolicy,
			IPFamilies:            ipFamilies,
			IPFamilyPolicy:        &ipFamilyPolicy,
			Ports: []corev1.ServicePort{
				{
					Port:       OLSAppRedisServicePort,
					Protocol:   corev1.ProtocolTCP,
					Name:       "server",
					TargetPort: intstr.Parse("server"),
				},
			},
			Selector:        DeploymentSelectorLabels,
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            corev1.ServiceTypeClusterIP,
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
			Name:      cr.Spec.OLSConfig.ConversationCache.Redis.CredentialsSecretRef.Name,
			Namespace: cr.Namespace,
			Labels:    DeploymentSelectorLabels,
			Annotations: map[string]string{
				OLSRedisSecretHashKey: passwordHash,
			},
		},
		Data: map[string][]byte{
			OLSRedisSecretKeyName: []byte(encodedPassword),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.Scheme); err != nil {
		return nil, err
	}

	return &secret, nil
}
