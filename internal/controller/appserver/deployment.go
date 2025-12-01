package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func getOLSServerReplicas(cr *olsv1alpha1.OLSConfig) *int32 {
	if cr.Spec.OLSConfig.DeploymentConfig.Replicas != nil && *cr.Spec.OLSConfig.DeploymentConfig.Replicas >= 0 {
		return cr.Spec.OLSConfig.DeploymentConfig.Replicas
	}
	// default number of replicas.
	defaultReplicas := int32(1)
	return &defaultReplicas
}

func getOLSServerResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Resources,
		&corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("1Gi")},
			Claims:   []corev1.ResourceClaim{},
		},
	)
}

func getOLSDataCollectorResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.DataCollectorContainer.Resources,
		&corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
			Claims:   []corev1.ResourceClaim{},
		},
	)
}

func getOLSMCPServerResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.MCPServerContainer.Resources,
		&corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
			Claims:   []corev1.ResourceClaim{},
		},
	)
}

func GenerateOLSDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	ctx := context.Background()
	// mount points of API key secret
	const OLSConfigMountPath = "/etc/ols"
	const OLSConfigVolumeName = "cm-olsconfig"
	const OLSUserDataVolumeName = "ols-user-data"

	revisionHistoryLimit := int32(1)
	volumeDefaultMode := utils.VolumeDefaultMode

	dataCollectorEnabled, err := dataCollectorEnabled(r, cr)
	if err != nil {
		return nil, err
	}

	// certificates mount paths
	AdditionalCAMountPath := path.Join(utils.OLSAppCertsMountRoot, utils.AppAdditionalCACertDir)
	UserCAMountPath := path.Join(utils.OLSAppCertsMountRoot, utils.UserCACertDir)

	// Container ports
	ports := []corev1.ContainerPort{
		{
			ContainerPort: utils.OLSAppServerContainerPort,
			Name:          "https",
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// Initialize volumes and volumeMounts slices
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	// Add external LLM provider and TLS secrets - create both volumes and volume mounts in single pass
	_ = utils.ForEachExternalSecret(cr, func(name, source string) error {
		var mountPath string
		if strings.HasPrefix(source, "llm-provider-") {
			mountPath = path.Join(utils.APIKeyMountRoot, name)
		} else if source == "tls" {
			mountPath = path.Join(utils.OLSAppCertsMountRoot, name)
		} else {
			// MCP header secrets are handled separately below
			return nil
		}

		volumes = append(volumes, corev1.Volume{
			Name: "secret-" + name,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  name,
					DefaultMode: &volumeDefaultMode,
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "secret-" + name,
			MountPath: mountPath,
			ReadOnly:  true,
		})
		return nil
	})

	// Postgres secret volume and mount (operator-owned, not external)
	postgresCredentialsMountPath := path.Join(utils.CredentialsMountRoot, utils.PostgresSecretName)
	volumes = append(volumes, corev1.Volume{
		Name: "secret-" + utils.PostgresSecretName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  utils.PostgresSecretName,
				DefaultMode: &volumeDefaultMode,
			},
		},
	})
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "secret-" + utils.PostgresSecretName,
		MountPath: postgresCredentialsMountPath,
		ReadOnly:  true,
	})

	// TLS secret volume and mount - handle operator-generated cert if no user cert provided
	if cr.Spec.OLSConfig.TLSConfig == nil || cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name == "" {
		// Use operator-generated certificate
		tlsMountPath := path.Join(utils.OLSAppCertsMountRoot, utils.OLSCertsSecretName)
		volumes = append(volumes, corev1.Volume{
			Name: "secret-" + utils.OLSCertsSecretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  utils.OLSCertsSecretName,
					DefaultMode: &volumeDefaultMode,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "secret-" + utils.OLSCertsSecretName,
			MountPath: tlsMountPath,
			ReadOnly:  true,
		})
	}

	// OLS config map volume and mount
	olsConfigVolume := corev1.Volume{
		Name: OLSConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: utils.OLSConfigCmName,
				},
				DefaultMode: &volumeDefaultMode,
			},
		},
	}
	volumes = append(volumes, olsConfigVolume)
	olsConfigVolumeMount := corev1.VolumeMount{
		Name:      OLSConfigVolumeName,
		MountPath: OLSConfigMountPath,
		ReadOnly:  true,
	}
	volumeMounts = append(volumeMounts, olsConfigVolumeMount)

	// Data collector volumes and mounts (if enabled)
	if dataCollectorEnabled {
		olsUserDataVolume := corev1.Volume{
			Name: OLSUserDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		volumes = append(volumes, olsUserDataVolume)

		olsUserDataVolumeMount := corev1.VolumeMount{
			Name:      OLSUserDataVolumeName,
			MountPath: utils.OLSUserDataMountPath,
		}
		volumeMounts = append(volumeMounts, olsUserDataVolumeMount)

		// Add exporter config volume and mount
		exporterConfigVolume := corev1.Volume{
			Name: utils.ExporterConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: utils.ExporterConfigCmName,
					},
					DefaultMode: &volumeDefaultMode,
				},
			},
		}
		volumes = append(volumes, exporterConfigVolume)

		exporterConfigVolumeMount := corev1.VolumeMount{
			Name:      utils.ExporterConfigVolumeName,
			MountPath: utils.ExporterConfigMountPath,
			ReadOnly:  true,
		}
		volumeMounts = append(volumeMounts, exporterConfigVolumeMount)
	}

	// Mount "kube-root-ca.crt" configmap
	certVolume := corev1.Volume{
		Name: utils.OpenShiftCAVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "kube-root-ca.crt",
				},
				DefaultMode: &volumeDefaultMode,
			},
		},
	}

	// Create certificates volume
	certBundleVolume := corev1.Volume{
		Name: utils.CertBundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volumes = append(volumes, certVolume, certBundleVolume)

	// Volumemount OpenShift certificates configmap
	openShiftCAVolumeMount := corev1.VolumeMount{
		Name:      utils.OpenShiftCAVolumeName,
		MountPath: AdditionalCAMountPath,
		ReadOnly:  true,
	}

	certBundleVolumeMount := corev1.VolumeMount{
		Name:      utils.CertBundleVolumeName,
		MountPath: path.Join(utils.OLSAppCertsMountRoot, utils.CertBundleVolumeName),
	}
	volumeMounts = append(volumeMounts, openShiftCAVolumeMount, certBundleVolumeMount)

	// User provided CA certificates - create both volumes and volume mounts in single pass
	_ = utils.ForEachExternalConfigMap(cr, func(name, source string) error {
		var volumeName, mountPath string
		switch source {
		case "additional-ca":
			volumeName = utils.AdditionalCAVolumeName
			mountPath = UserCAMountPath
		case "proxy-ca":
			volumeName = utils.ProxyCACertVolumeName
			mountPath = path.Join(utils.OLSAppCertsMountRoot, utils.ProxyCACertVolumeName)
		default:
			return nil
		}

		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: name},
					DefaultMode:          &volumeDefaultMode,
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: mountPath,
			ReadOnly:  true,
		})
		return nil
	})

	// RAG volume
	if len(cr.Spec.OLSConfig.RAG) > 0 {
		ragVolume := generateRAGVolume()
		volumes = append(volumes, ragVolume)
	}

	// Postgres CA volume
	volumes = append(volumes, utils.GetPostgresCAConfigVolume())

	volumes = append(volumes,
		corev1.Volume{
			Name: utils.TmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	)

	if len(cr.Spec.OLSConfig.RAG) > 0 {
		ragVolumeMounts := generateRAGVolumeMount()
		volumeMounts = append(volumeMounts, ragVolumeMounts)
	}

	volumeMounts = append(volumeMounts,
		utils.GetPostgresCAVolumeMount(path.Join(utils.OLSAppCertsMountRoot, utils.PostgresCertsSecretName, utils.PostgresCAVolume)),
		corev1.VolumeMount{
			Name:      utils.TmpVolumeName,
			MountPath: utils.TmpVolumeMountPath,
		},
	)

	// mount the volumes and add Volume mounts for the MCP server headers
	_ = utils.ForEachExternalSecret(cr, func(name, source string) error {
		if strings.HasPrefix(source, "mcp-") {
			volumes = append(volumes, corev1.Volume{
				Name: "header-" + name,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  name,
						DefaultMode: &volumeDefaultMode,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "header-" + name,
				MountPath: path.Join(utils.MCPHeadersMountRoot, name),
				ReadOnly:  true,
			})
		}
		return nil
	})

	initContainers := []corev1.Container{}
	if len(cr.Spec.OLSConfig.RAG) > 0 {
		ragInitContainers := GenerateRAGInitContainers(cr)
		initContainers = append(initContainers, ragInitContainers...)
	}

	replicas := getOLSServerReplicas(cr)
	ols_server_resources := getOLSServerResources(cr)
	data_collector_resources := getOLSDataCollectorResources(cr)
	mcp_server_resources := getOLSMCPServerResources(cr)

	// Get ResourceVersions for tracking - these resources should already exist
	// If they don't exist, we'll get empty strings which is fine for initial creation
	configMapResourceVersion, _ := utils.GetConfigMapResourceVersion(r, ctx, utils.OLSConfigCmName)
	secretResourceVersion, _ := utils.GetSecretResourceVersion(r, ctx, utils.PostgresSecretName)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OLSAppServerDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GenerateAppServerSelectorLabels(),
			Annotations: map[string]string{
				utils.OLSConfigMapResourceVersionAnnotation:   configMapResourceVersion,
				utils.PostgresSecretResourceVersionAnnotation: secretResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: utils.GenerateAppServerSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: utils.GenerateAppServerSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "lightspeed-service-api",
							Image:           r.GetAppServerImage(),
							ImagePullPolicy: corev1.PullAlways,
							Ports:           ports,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								ReadOnlyRootFilesystem:   &[]bool{true}[0],
							},
							VolumeMounts: volumeMounts,
							Env: append(utils.GetProxyEnvVars(), corev1.EnvVar{
								Name:  "OLS_CONFIG_FILE",
								Value: path.Join(OLSConfigMountPath, utils.OLSConfigFilename),
							}),
							Resources: *ols_server_resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/readiness",
										Port:   intstr.FromString("https"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       30,
								TimeoutSeconds:      30,
								FailureThreshold:    15,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/liveness",
										Port:   intstr.FromString("https"),
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       30,
								TimeoutSeconds:      30,
								FailureThreshold:    15,
							},
						},
					},
					InitContainers:     initContainers,
					Volumes:            volumes,
					ServiceAccountName: utils.OLSAppServerServiceAccountName,
				},
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
		},
	}

	if cr.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = cr.Spec.OLSConfig.DeploymentConfig.APIContainer.NodeSelector
	}
	if cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations != nil {
		deployment.Spec.Template.Spec.Tolerations = cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Tolerations
	}

	if err := controllerutil.SetControllerReference(cr, &deployment, r.GetScheme()); err != nil {
		return nil, err
	}

	// Add additional containers in a consistent order:
	// 1. Data collector container (if enabled)
	// 2. MCP server container (if enabled)

	if dataCollectorEnabled {
		// Add data exporter container
		logLevel := cr.Spec.OLSDataCollectorConfig.LogLevel
		if logLevel == "" {
			logLevel = "INFO"
		}
		exporterContainer := corev1.Container{
			Name:            "lightspeed-to-dataverse-exporter",
			Image:           r.GetDataverseExporterImage(),
			ImagePullPolicy: corev1.PullAlways,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
			},
			VolumeMounts: volumeMounts,
			// running in openshift mode ensures that cluster_id is set
			// as identity_id
			Args: []string{
				"--mode",
				"openshift",
				"--config",
				path.Join(utils.ExporterConfigMountPath, utils.ExporterConfigFilename),
				"--log-level",
				logLevel,
				"--data-dir",
				utils.OLSUserDataMountPath,
			},
			Resources: *data_collector_resources,
		}
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, exporterContainer)
	}

	// Add OpenShift MCP server sidecar container if introspection is enabled
	if cr.Spec.OLSConfig.IntrospectionEnabled {
		openshiftMCPServerSidecarContainer := corev1.Container{
			Name:            "openshift-mcp-server",
			Image:           r.GetOpenShiftMCPServerImage(),
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
			},
			VolumeMounts: volumeMounts,
			Command:      []string{"/openshift-mcp-server", "--read-only", "--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerPort)},
			Resources:    *mcp_server_resources,
		}
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, openshiftMCPServerSidecarContainer)
	}

	return &deployment, nil
}

// updateOLSDeployment updates the deployment based on CustomResource configuration.
func updateOLSDeployment(r reconciler.Reconciler, ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	// Step 1: Check if deployment spec has changed
	utils.SetDefaults_Deployment(desiredDeployment)
	changed := !utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec)

	// Step 2: Check ConfigMap and Secret ResourceVersions
	// Check if OLS ConfigMap ResourceVersion has changed
	currentConfigMapVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.OLSConfigCmName)
	if err != nil {
		r.GetLogger().Info("failed to get ConfigMap ResourceVersion", "error", err)
		changed = true
	} else {
		storedConfigMapVersion := existingDeployment.Annotations[utils.OLSConfigMapResourceVersionAnnotation]
		if storedConfigMapVersion != currentConfigMapVersion {
			changed = true
		}
	}

	// Check if Postgres Secret ResourceVersion has changed
	currentSecretVersion, err := utils.GetSecretResourceVersion(r, ctx, utils.PostgresSecretName)
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
	existingDeployment.Annotations[utils.OLSConfigMapResourceVersionAnnotation] = desiredDeployment.Annotations[utils.OLSConfigMapResourceVersionAnnotation]
	existingDeployment.Annotations[utils.PostgresSecretResourceVersionAnnotation] = desiredDeployment.Annotations[utils.PostgresSecretResourceVersionAnnotation]

	r.GetLogger().Info("updating OLS deployment", "name", existingDeployment.Name)

	if err := RestartAppServer(r, ctx, existingDeployment); err != nil {
		return err
	}

	return nil
}

func telemetryEnabled(r reconciler.Reconciler) (bool, error) {
	// Telemetry enablement is determined by the presence of the telemetry pull secret
	// the presence of the field '.auths."cloud.openshift.com"' indicates that telemetry is enabled
	// use this command to check in an Openshift cluster
	// oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' | jq '.auths."cloud.openshift.com"'
	pullSecret := &corev1.Secret{}
	err := r.Get(context.Background(), client.ObjectKey{Namespace: utils.TelemetryPullSecretNamespace, Name: utils.TelemetryPullSecretName}, pullSecret)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	dockerconfigjson, ok := pullSecret.Data[".dockerconfigjson"]
	if !ok {
		return false, fmt.Errorf("pull secret does not contain .dockerconfigjson")
	}

	dockerconfigjsonDecoded := map[string]interface{}{}
	err = json.Unmarshal(dockerconfigjson, &dockerconfigjsonDecoded)
	if err != nil {
		return false, err
	}

	_, ok = dockerconfigjsonDecoded["auths"].(map[string]interface{})["cloud.openshift.com"]
	return ok, nil

}

func dataCollectorEnabled(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (bool, error) {
	// data collector is enabled in OLS configuration
	configEnabled := !cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled || !cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled
	telemetryEnabled, err := telemetryEnabled(r)
	if err != nil {
		return false, err
	}
	return configEnabled && telemetryEnabled, nil
}

// RestartAppServer triggers a rolling restart of the app server deployment by updating its pod template annotation.
// This is useful when configuration changes require a pod restart (e.g., ConfigMap or Secret updates).
func RestartAppServer(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	var dep *appsv1.Deployment
	var err error

	// If deployment is provided, use it; otherwise fetch it
	if len(deployment) > 0 && deployment[0] != nil {
		dep = deployment[0]
	} else {
		// Get the app server deployment
		dep = &appsv1.Deployment{}
		err = r.Get(ctx, client.ObjectKey{Name: utils.OLSAppServerDeploymentName, Namespace: r.GetNamespace()}, dep)
		if err != nil {
			r.GetLogger().Info("failed to get deployment", "deploymentName", utils.OLSAppServerDeploymentName, "error", err)
			return err
		}
	}

	// Initialize annotations map if empty
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}

	// Bump the annotation to trigger a rolling update (new template hash)
	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	// Update the deployment
	r.GetLogger().Info("triggering app server rolling restart", "deployment", dep.Name)
	err = r.Update(ctx, dep)
	if err != nil {
		r.GetLogger().Info("failed to update deployment", "deploymentName", dep.Name, "error", err)
		return err
	}

	return nil
}
