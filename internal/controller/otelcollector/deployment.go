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
		},
	)
}

// getOtelDataverseExporterResources uses the existing data-collector override and defaults.
func getOtelDataverseExporterResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.DataCollectorContainer.Resources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Claims: []corev1.ResourceClaim{},
		},
	)
}

// This projection supplies the conventional in-cluster auth files while automount stays disabled.
func generateOtelDataverseExporterAuthVolume() corev1.Volume {
	defaultMode := utils.VolumeRestrictedMode
	expirationSeconds := int64(3600)
	return corev1.Volume{
		Name: utils.OtelDataverseExporterAuthVolumeName,
		VolumeSource: corev1.VolumeSource{
			Projected: &corev1.ProjectedVolumeSource{
				DefaultMode: &defaultMode,
				Sources: []corev1.VolumeProjection{
					{
						ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Path:              "token",
							ExpirationSeconds: &expirationSeconds,
						},
					},
					{
						ConfigMap: &corev1.ConfigMapProjection{
							LocalObjectReference: corev1.LocalObjectReference{Name: "kube-root-ca.crt"},
							Items: []corev1.KeyToPath{
								{Key: "ca.crt", Path: "ca.crt"},
							},
						},
					},
					{
						DownwardAPI: &corev1.DownwardAPIProjection{
							Items: []corev1.DownwardAPIVolumeFile{
								{
									Path: "namespace",
									FieldRef: &corev1.ObjectFieldSelector{
										APIVersion: "v1",
										FieldPath:  "metadata.namespace",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// GenerateOtelCollectorDeployment generates the OTEL Collector Deployment.
// Postgres DSN, admin port, and Postgres wait init are always present: clients call
// postgres_admin regardless of spec.audit.logging. Only the runtime ConfigMap pipelines
// change with logging.
func GenerateOtelCollectorDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	revisionHistoryLimit := int32(1)
	runAsNonRoot := true
	dataCollectionEnabled := !cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled

	configMapResourceVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.OtelCollectorConfigMapName)
	if err != nil {
		return nil, err
	}
	exporterEnabled, err := dataverseExporterEnabled(r, ctx, cr)
	if err != nil {
		return nil, fmt.Errorf("failed to determine OTEL Dataverse exporter enablement: %w", err)
	}

	var exporterConfigMapResourceVersion string
	if exporterEnabled {
		exporterConfigMapResourceVersion, err = utils.GetConfigMapResourceVersion(r, ctx, utils.OtelDataverseExporterConfigMapName)
		if err != nil {
			return nil, fmt.Errorf("failed to get OTEL Dataverse exporter ConfigMap resource version: %w", err)
		}
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
		{
			Name: utils.OtelCollectorServiceCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: utils.OLSCAConfigMap},
					DefaultMode:          &[]int32{utils.VolumeDefaultMode}[0],
				},
			},
		},
	}
	if dataCollectionEnabled {
		dataCollectionSizeLimit := resource.MustParse(utils.OtelCollectorDataCollectionSizeLimitDefault)
		volumes = append(volumes, corev1.Volume{
			Name: utils.OtelCollectorDataCollectionVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: &dataCollectionSizeLimit,
				},
			},
		})
	}
	if exporterEnabled {
		configMapMode := utils.VolumeRestrictedMode
		volumes = append(volumes,
			corev1.Volume{
				Name: utils.OtelDataverseExporterConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: utils.OtelDataverseExporterConfigMapName},
						DefaultMode:          &configMapMode,
					},
				},
			},
			corev1.Volume{
				Name: utils.OtelDataverseExporterStateVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			generateOtelDataverseExporterAuthVolume(),
		)
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
		{
			Name:      utils.OtelCollectorServiceCAVolumeName,
			MountPath: utils.OtelCollectorServiceCAMountPath,
			ReadOnly:  true,
		},
	}
	if dataCollectionEnabled {
		// Keep FileExporter data separate from the file_storage exporter queue.
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      utils.OtelCollectorDataCollectionVolumeName,
			MountPath: utils.OtelCollectorDataCollectionMountPath,
		})
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
		{
			Name:          "admin",
			ContainerPort: utils.OtelCollectorAdminPort,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "metrics",
			ContainerPort: utils.OtelCollectorMetricsPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	envVars := append([]corev1.EnvVar{}, utils.GetProxyEnvVars()...)
	envVars = append(envVars, corev1.EnvVar{
		Name: utils.OtelCollectorPostgresConnectionStringEnvVar,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: utils.OtelCollectorPostgresDSNSecretName},
				Key:                  utils.OtelCollectorPostgresConnectionStringSecretKey,
			},
		},
	})
	if cr.Spec.Audit.TracingEndpoint != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  utils.OtelCollectorTracesBackendEndpointEnvVar,
			Value: cr.Spec.Audit.TracingEndpoint,
		})
	}

	initContainers := []corev1.Container{
		utils.GeneratePostgresWaitInitContainer(r.GetPostgresImage()),
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
					AutomountServiceAccountToken: utils.BoolPtr(false),
					ServiceAccountName:           utils.OtelCollectorServiceAccountName,
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
	if exporterEnabled {
		deployment.Annotations[utils.OtelDataverseExporterConfigMapResourceVersionAnnotation] = exporterConfigMapResourceVersion
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, corev1.Container{
			Name:            utils.DataverseExporterContainerName,
			Image:           r.GetDataverseExporterImage(),
			ImagePullPolicy: corev1.PullAlways,
			Args: []string{
				"--mode",
				"openshift",
				"--config",
				utils.OtelDataverseExporterConfigPath,
			},
			Env:             append([]corev1.EnvVar{}, utils.GetProxyEnvVars()...),
			SecurityContext: utils.RestrictedContainerSecurityContext(),
			Resources:       *getOtelDataverseExporterResources(cr),
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      utils.OtelDataverseExporterConfigVolumeName,
					MountPath: utils.OtelDataverseExporterConfigMountPath,
					ReadOnly:  true,
				},
				{
					Name:      utils.OtelCollectorDataCollectionVolumeName,
					MountPath: utils.OtelDataverseExporterDataMountPath,
					ReadOnly:  true,
				},
				{
					Name:      utils.OtelDataverseExporterStateVolumeName,
					MountPath: utils.OtelDataverseExporterStateMountPath,
				},
				{
					Name:      utils.OtelDataverseExporterAuthVolumeName,
					MountPath: utils.OtelDataverseExporterAuthMountPath,
					ReadOnly:  true,
				},
			},
		})
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

	trackedConfigMaps := []struct {
		name       string
		annotation string
	}{
		{
			name:       utils.OtelCollectorConfigMapName,
			annotation: utils.OtelCollectorConfigMapResourceVersionAnnotation,
		},
		{
			name:       utils.OtelDataverseExporterConfigMapName,
			annotation: utils.OtelDataverseExporterConfigMapResourceVersionAnnotation,
		},
	}
	for _, configMap := range trackedConfigMaps {
		storedVersion := existingDeployment.Annotations[configMap.annotation]
		desiredVersion, desired := desiredDeployment.Annotations[configMap.annotation]
		if !desired {
			if storedVersion != "" {
				changed = true
			}
			continue
		}

		currentVersion, err := utils.GetConfigMapResourceVersion(r, ctx, configMap.name)
		if err != nil {
			r.GetLogger().Info("failed to get OTEL ConfigMap resource version", "configmap", configMap.name, "error", err)
			changed = true
			continue
		}
		if storedVersion != currentVersion || desiredVersion != currentVersion {
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
	for _, configMap := range trackedConfigMaps {
		if version, desired := desiredDeployment.Annotations[configMap.annotation]; desired {
			existingDeployment.Annotations[configMap.annotation] = version
		} else {
			delete(existingDeployment.Annotations, configMap.annotation)
		}
	}

	r.GetLogger().Info("updating OTEL Collector deployment", "name", existingDeployment.Name)
	return RestartOtelCollector(r, ctx, existingDeployment)
}
