package otelcollector

import (
	"context"
	"fmt"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

const otelCollectorFileStorageVolumeName = "file-storage"

func getOtelCollectorResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.OtelCollector.Resources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	)
}

// GenerateOtelCollectorDeployment generates the OTEL Collector Deployment.
func GenerateOtelCollectorDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	loggingEnabled := utils.BoolDeref(cr.Spec.Audit.Logging, true)
	revisionHistoryLimit := int32(1)
	runAsNonRoot := true

	configMapResourceVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.OtelCollectorConfigMapName)
	if err != nil {
		return nil, err
	}

	volumes := []corev1.Volume{
		{
			Name: utils.OtelCollectorConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: utils.OtelCollectorConfigMapName},
				},
			},
		},
		{
			Name: utils.OtelCollectorServingCertVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.OtelCollectorCertsSecretName,
					DefaultMode: &[]int32{utils.VolumeRestrictedMode}[0],
				},
			},
		},
		// file_storage queue uses emptyDir: survives container restarts within a pod, not rollouts.
		// If queue durability across rollouts matters for audit, go straight to StatefulSet with
		// volumeClaimTemplates; don't switch to StatefulSet just for emptyDir.
		{
			Name: otelCollectorFileStorageVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: resource.NewQuantity(500*1024*1024, resource.BinarySI),
				},
			},
		},
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      utils.OtelCollectorConfigVolumeName,
			MountPath: utils.OtelCollectorConfigVolumeMountPath,
			ReadOnly:  true,
		},
		{
			Name:      utils.OtelCollectorServingCertVolumeName,
			MountPath: utils.OtelCollectorServingCertMountPath,
			ReadOnly:  true,
		},
		{
			Name:      otelCollectorFileStorageVolumeName,
			MountPath: utils.OtelCollectorFileStorageMountPath,
		},
	}

	ports := []corev1.ContainerPort{
		{
			Name:          "otlp-grpc",
			ContainerPort: utils.OtelCollectorGRPCPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "otlp-http",
			ContainerPort: utils.OtelCollectorHTTPPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	if loggingEnabled {
		ports = append(ports, corev1.ContainerPort{
			Name:          "admin",
			ContainerPort: utils.OtelCollectorAdminPort,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	envVars := append([]corev1.EnvVar{}, utils.GetProxyEnvVars()...)
	if loggingEnabled {
		envVars = append(envVars, corev1.EnvVar{
			Name: utils.OtelCollectorPostgresConnectionStringEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: utils.OtelCollectorPostgresDSNSecretName},
					Key:                  utils.OtelCollectorPostgresConnectionStringSecretKey,
				},
			},
		})
	}
	if cr.Spec.Audit.TracingEndpoint != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  utils.OtelCollectorTracesBackendEndpointEnvVar,
			Value: cr.Spec.Audit.TracingEndpoint,
		})
	}

	initContainers := []corev1.Container{}
	if loggingEnabled {
		initContainers = append(initContainers, utils.GeneratePostgresWaitInitContainer(r.GetPostgresImage()))
	}

	healthCheckPort := intstr.FromInt32(utils.OtelCollectorHealthCheckPort)
	configPath := path.Join(utils.OtelCollectorConfigVolumeMountPath, utils.OtelCollectorConfigMapDataKey)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OtelCollectorDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateOtelCollectorSelectorLabels(),
			Annotations: map[string]string{
				utils.OtelCollectorConfigMapResourceVersionAnnotation: configMapResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: utils.GenerateOtelCollectorSelectorLabels(),
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: utils.GenerateOtelCollectorSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: utils.OtelCollectorServiceAccountName,
					// No explicit UID/GID — OpenShift assigns from the namespace range via restricted SCC.
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Name:            utils.OtelCollectorContainerName,
							Image:           r.GetOtelCollectorImage(),
							ImagePullPolicy: corev1.PullAlways,
							Args:            []string{"--config=" + configPath},
							SecurityContext: utils.RestrictedContainerSecurityContext(),
							Ports:           ports,
							Env:             envVars,
							Resources:       *getOtelCollectorResources(cr),
							VolumeMounts:    volumeMounts,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/",
										Port:   healthCheckPort,
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       15,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/",
										Port:   healthCheckPort,
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
							},
						},
					},
					Volumes: volumes,
				},
			},
		},
	}

	utils.ApplyPodDeploymentConfig(deployment, cr.Spec.OLSConfig.DeploymentConfig.OtelCollector, false)

	if err := controllerutil.SetControllerReference(cr, deployment, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOtelCollectorDeploymentOwnerReference, err)
	}

	return deployment, nil
}

// UpdateOtelCollectorDeployment updates the collector deployment when the pod spec or owned ConfigMap changes.
func UpdateOtelCollectorDeployment(r reconciler.Reconciler, ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	utils.SetDefaults_Deployment(desiredDeployment)
	changed := !utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec, false)

	currentConfigMapVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.OtelCollectorConfigMapName)
	if err != nil {
		r.GetLogger().Info("failed to get OTEL Collector ConfigMap ResourceVersion", "error", err)
		changed = true
	} else {
		storedConfigMapVersion := existingDeployment.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation]
		if storedConfigMapVersion != currentConfigMapVersion {
			changed = true
		}
	}

	if !changed {
		return nil
	}

	existingDeployment.Spec = desiredDeployment.Spec
	if existingDeployment.Annotations == nil {
		existingDeployment.Annotations = make(map[string]string)
	}
	existingDeployment.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation] =
		desiredDeployment.Annotations[utils.OtelCollectorConfigMapResourceVersionAnnotation]

	r.GetLogger().Info("updating OTEL Collector deployment", "name", existingDeployment.Name)
	return RestartOtelCollector(r, ctx, existingDeployment)
}
