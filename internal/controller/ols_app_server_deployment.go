package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
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
	if cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.APIContainer.Resources
	}
	// default resources.
	defaultResources := &corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("1Gi")},
		Claims:   []corev1.ResourceClaim{},
	}

	return defaultResources
}

func getOLSDataCollectorResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	if cr.Spec.OLSConfig.DeploymentConfig.DataCollectorContainer.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.DataCollectorContainer.Resources
	}
	// default resources.
	defaultResources := &corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
		Claims:   []corev1.ResourceClaim{},
	}

	return defaultResources
}

func getOLSMCPServerResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	if cr.Spec.OLSConfig.DeploymentConfig.MCPServerContainer.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.MCPServerContainer.Resources
	}
	defaultResources := &corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("200Mi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
		Claims:   []corev1.ResourceClaim{},
	}

	return defaultResources
}

func (r *OLSConfigReconciler) generateOLSDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	// mount points of API key secret
	const OLSConfigMountPath = "/etc/ols"
	const OLSConfigVolumeName = "cm-olsconfig"
	const OLSUserDataVolumeName = "ols-user-data"

	revisionHistoryLimit := int32(1)
	volumeDefaultMode := int32(420)

	dataCollectorEnabled, err := r.dataCollectorEnabled(cr)
	if err != nil {
		return nil, err
	}

	// map from secret name to secret mount path
	secretMounts := map[string]string{}
	for _, provider := range cr.Spec.LLMConfig.Providers {
		credentialMountPath := path.Join(APIKeyMountRoot, provider.CredentialsSecretRef.Name)
		secretMounts[provider.CredentialsSecretRef.Name] = credentialMountPath
	}

	// Postgres Volume
	postgresSecretName := PostgresSecretName
	if cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret != "" {
		postgresSecretName = cr.Spec.OLSConfig.ConversationCache.Postgres.CredentialsSecret
	}
	postgresCredentialsMountPath := path.Join(CredentialsMountRoot, postgresSecretName)
	secretMounts[postgresSecretName] = postgresCredentialsMountPath

	// TLS volume
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		secretMounts[cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name] = path.Join(OLSAppCertsMountRoot, cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name)
	} else {
		secretMounts[OLSCertsSecretName] = path.Join(OLSAppCertsMountRoot, OLSCertsSecretName)
	}

	// certificates mount paths
	AdditionalCAMountPath := path.Join(OLSAppCertsMountRoot, AppAdditionalCACertDir)
	UserCAMountPath := path.Join(OLSAppCertsMountRoot, UserCACertDir)

	// Container ports
	ports := []corev1.ContainerPort{
		{
			ContainerPort: OLSAppServerContainerPort,
			Name:          "https",
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// declare api key secrets and OLS config map as volumes to the pod
	volumes := []corev1.Volume{}
	for secretName := range secretMounts {
		volume := corev1.Volume{
			Name: "secret-" + secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  secretName,
					DefaultMode: &volumeDefaultMode,
				},
			},
		}
		volumes = append(volumes, volume)
	}
	olsConfigVolume := corev1.Volume{
		Name: OLSConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: OLSConfigCmName,
				},
				DefaultMode: &volumeDefaultMode,
			},
		},
	}
	volumes = append(volumes, olsConfigVolume)
	if dataCollectorEnabled {
		olsUserDataVolume := corev1.Volume{
			Name: OLSUserDataVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		volumes = append(volumes, olsUserDataVolume)

		// Add exporter config volume
		exporterConfigVolume := corev1.Volume{
			Name: ExporterConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ExporterConfigCmName,
					},
					DefaultMode: &volumeDefaultMode,
				},
			},
		}
		volumes = append(volumes, exporterConfigVolume)
	}

	// Mount "kube-root-ca.crt" configmap
	certVolume := corev1.Volume{
		Name: OpenShiftCAVolumeName,
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
		Name: CertBundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volumes = append(volumes, certVolume, certBundleVolume)

	// User provided additional CA certificates
	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		additionalCAVolume := corev1.Volume{
			Name: AdditionalCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *cr.Spec.OLSConfig.AdditionalCAConfigMapRef,
					DefaultMode:          &volumeDefaultMode,
				},
			},
		}
		volumes = append(volumes, additionalCAVolume)
	}

	// Proxy CA certificates
	if cr.Spec.OLSConfig.ProxyConfig != nil && cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef != nil {
		proxyCACertVolume := corev1.Volume{
			Name: ProxyCACertVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: *cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef,
					DefaultMode:          &volumeDefaultMode,
				},
			},
		}
		volumes = append(volumes, proxyCACertVolume)
	}

	// RAG volume
	if len(cr.Spec.OLSConfig.RAG) > 0 {
		ragVolume := r.generateRAGVolume()
		volumes = append(volumes, ragVolume)
	}

	// Postgres CA volume
	volumes = append(volumes, getPostgresCAConfigVolume())

	volumes = append(volumes,
		corev1.Volume{
			Name: TmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	)

	// mount the volumes of api keys secrets and OLS config map to the container
	volumeMounts := []corev1.VolumeMount{}
	for secretName, mountPath := range secretMounts {
		volumeMount := corev1.VolumeMount{
			Name:      "secret-" + secretName,
			MountPath: mountPath,
			ReadOnly:  true,
		}
		volumeMounts = append(volumeMounts, volumeMount)
	}
	olsConfigVolumeMount := corev1.VolumeMount{
		Name:      OLSConfigVolumeName,
		MountPath: OLSConfigMountPath,
		ReadOnly:  true,
	}
	volumeMounts = append(volumeMounts, olsConfigVolumeMount)

	olsUserDataVolumeMount := corev1.VolumeMount{
		Name:      OLSUserDataVolumeName,
		MountPath: OLSUserDataMountPath,
	}
	exporterConfigVolumeMount := corev1.VolumeMount{
		Name:      ExporterConfigVolumeName,
		MountPath: ExporterConfigMountPath,
		ReadOnly:  true,
	}

	if dataCollectorEnabled {
		volumeMounts = append(volumeMounts, olsUserDataVolumeMount, exporterConfigVolumeMount)
	}

	// Volumemount OpenShift certificates configmap
	openShiftCAVolumeMount := corev1.VolumeMount{
		Name:      OpenShiftCAVolumeName,
		MountPath: AdditionalCAMountPath,
		ReadOnly:  true,
	}

	certBundleVolumeMount := corev1.VolumeMount{
		Name:      CertBundleVolumeName,
		MountPath: path.Join(OLSAppCertsMountRoot, CertBundleDir),
	}
	volumeMounts = append(volumeMounts, openShiftCAVolumeMount, certBundleVolumeMount)

	if cr.Spec.OLSConfig.AdditionalCAConfigMapRef != nil {
		additionalCAVolumeMount := corev1.VolumeMount{
			Name:      AdditionalCAVolumeName,
			MountPath: UserCAMountPath,
			ReadOnly:  true,
		}
		volumeMounts = append(volumeMounts, additionalCAVolumeMount)
	}

	if cr.Spec.OLSConfig.ProxyConfig != nil && cr.Spec.OLSConfig.ProxyConfig.ProxyCACertificateRef != nil {
		proxyCACertVolumeMount := corev1.VolumeMount{
			Name:      ProxyCACertVolumeName,
			MountPath: path.Join(OLSAppCertsMountRoot, ProxyCACertVolumeName),
			ReadOnly:  true,
		}
		volumeMounts = append(volumeMounts, proxyCACertVolumeMount)
	}

	if len(cr.Spec.OLSConfig.RAG) > 0 {
		ragVolumeMounts := r.generateRAGVolumeMount()
		volumeMounts = append(volumeMounts, ragVolumeMounts)
	}

	volumeMounts = append(volumeMounts,
		getPostgresCAVolumeMount(path.Join(OLSAppCertsMountRoot, PostgresCertsSecretName, PostgresCAVolume)),
		corev1.VolumeMount{
			Name:      TmpVolumeName,
			MountPath: TmpVolumeMountPath,
		},
	)

	// mount the volumes and add Volume mounts for the MCP server headers
	for _, server := range cr.Spec.MCPServers {
		for _, v := range server.StreamableHTTP.Headers {
			if v == KUBERNETES_PLACEHOLDER {
				continue
			}
			volumes = append(volumes, corev1.Volume{
				Name: "header-" + v,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  v,
						DefaultMode: &volumeDefaultMode,
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      "header-" + v,
				MountPath: path.Join(MCPHeadersMountRoot, v),
				ReadOnly:  true,
			})
		}
	}

	initContainers := []corev1.Container{}
	if len(cr.Spec.OLSConfig.RAG) > 0 {
		ragInitContainers := r.generateRAGInitContainers(cr)
		initContainers = append(initContainers, ragInitContainers...)
	}

	replicas := getOLSServerReplicas(cr)
	ols_server_resources := getOLSServerResources(cr)
	data_collector_resources := getOLSDataCollectorResources(cr)
	mcp_server_resources := getOLSMCPServerResources(cr)

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OLSAppServerDeploymentName,
			Namespace: r.Options.Namespace,
			Labels:    generateAppServerSelectorLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: generateAppServerSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: generateAppServerSelectorLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "lightspeed-service-api",
							Image:           r.Options.LightspeedServiceImage,
							ImagePullPolicy: corev1.PullAlways,
							Ports:           ports,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								ReadOnlyRootFilesystem:   &[]bool{true}[0],
							},
							VolumeMounts: volumeMounts,
							Env: append(getProxyEnvVars(), corev1.EnvVar{
								Name:  "OLS_CONFIG_FILE",
								Value: path.Join(OLSConfigMountPath, OLSConfigFilename),
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
					ServiceAccountName: OLSAppServerServiceAccountName,
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

	if err := controllerutil.SetControllerReference(cr, &deployment, r.Scheme); err != nil {
		return nil, err
	}

	// Add additional containers in a consistent order:
	// 1. Data collector container (if enabled)
	// 2. MCP server container (if enabled)

	if dataCollectorEnabled {
		// Add data exporter container
		exporterContainer := corev1.Container{
			Name:            "lightspeed-to-dataverse-exporter",
			Image:           r.Options.DataverseExporterImage,
			ImagePullPolicy: corev1.PullAlways,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
			},
			VolumeMounts: volumeMounts,
			// running in openshift mode ensures that cluster_id is set
			// as identity_id
			// make logging configurable via config: OLS-2191
			Args: []string{
				"--mode",
				"openshift",
				"--config",
				path.Join(ExporterConfigMountPath, ExporterConfigFilename),
				"--log-level",
				"INFO",
				"--data-dir",
				OLSUserDataMountPath,
			},
			Resources: *data_collector_resources,
		}
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, exporterContainer)
	}

	// Add OpenShift MCP server sidecar container if introspection is enabled
	if cr.Spec.OLSConfig.IntrospectionEnabled {
		openshiftMCPServerSidecarContainer := corev1.Container{
			Name:            "openshift-mcp-server",
			Image:           r.Options.OpenShiftMCPServerImage,
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				ReadOnlyRootFilesystem:   &[]bool{true}[0],
			},
			VolumeMounts: volumeMounts,
			Command:      []string{"/openshift-mcp-server", "--read-only", "--port", fmt.Sprintf("%d", OpenShiftMCPServerPort)},
			Resources:    *mcp_server_resources,
		}
		deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, openshiftMCPServerSidecarContainer)
	}

	return &deployment, nil
}

// updateOLSDeployment updates the deployment based on CustomResource configuration.
func (r *OLSConfigReconciler) updateOLSDeployment(ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	changed := false

	// Validate deployment annotations.
	if existingDeployment.Annotations == nil ||
		existingDeployment.Annotations[OLSConfigHashKey] != r.stateCache[OLSConfigHashStateCacheKey] ||
		existingDeployment.Annotations[OLSAppTLSHashKey] != r.stateCache[OLSAppTLSHashStateCacheKey] ||
		existingDeployment.Annotations[LLMProviderHashKey] != r.stateCache[LLMProviderHashStateCacheKey] ||
		existingDeployment.Annotations[PostgresSecretHashKey] != r.stateCache[PostgresSecretHashStateCacheKey] {
		updateDeploymentAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			OLSAppTLSHashKey:      r.stateCache[OLSAppTLSHashStateCacheKey],
			LLMProviderHashKey:    r.stateCache[LLMProviderHashStateCacheKey],
			AdditionalCAHashKey:   r.stateCache[AdditionalCAHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		// update the deployment template annotation triggers the rolling update
		updateDeploymentTemplateAnnotations(existingDeployment, map[string]string{
			OLSConfigHashKey:      r.stateCache[OLSConfigHashStateCacheKey],
			OLSAppTLSHashKey:      r.stateCache[OLSAppTLSHashStateCacheKey],
			LLMProviderHashKey:    r.stateCache[LLMProviderHashStateCacheKey],
			AdditionalCAHashKey:   r.stateCache[AdditionalCAHashStateCacheKey],
			PostgresSecretHashKey: r.stateCache[PostgresSecretHashStateCacheKey],
		})
		changed = true
	}

	// Validate deployment replicas.
	if setDeploymentReplicas(existingDeployment, *desiredDeployment.Spec.Replicas) {
		changed = true
	}

	//validate deployment Tolerations
	if setTolerations(existingDeployment, desiredDeployment.Spec.Template.Spec.Tolerations) {
		changed = true
	}

	if setNodeSelector(existingDeployment, desiredDeployment.Spec.Template.Spec.NodeSelector) {
		changed = true
	}

	// Validate deployment volumes.
	if setVolumes(existingDeployment, desiredDeployment.Spec.Template.Spec.Volumes) {
		changed = true
	}

	// Validate volume mounts for a specific container in deployment.
	if volumeMountsChanged, err := setVolumeMounts(existingDeployment, desiredDeployment.Spec.Template.Spec.Containers[0].VolumeMounts, "lightspeed-service-api"); err != nil {
		return err
	} else if volumeMountsChanged {
		changed = true
	}

	// Validate deployment resources.
	if resourcesChanged, err := setDeploymentContainerResources(existingDeployment, &desiredDeployment.Spec.Template.Spec.Containers[0].Resources, "lightspeed-service-api"); err != nil {
		return err
	} else if resourcesChanged {
		changed = true
	}

	// validate volumes including token secrets and application config map
	if !podVolumeEqual(existingDeployment.Spec.Template.Spec.Volumes, desiredDeployment.Spec.Template.Spec.Volumes) {
		changed = true
		existingDeployment.Spec.Template.Spec.Volumes = desiredDeployment.Spec.Template.Spec.Volumes
		_, err := setDeploymentContainerVolumeMounts(existingDeployment, "lightspeed-service-api", desiredDeployment.Spec.Template.Spec.Containers[0].VolumeMounts)
		if err != nil {
			return err
		}
	}

	// validate container specs
	if !containersEqual(existingDeployment.Spec.Template.Spec.Containers, desiredDeployment.Spec.Template.Spec.Containers) {
		changed = true
		existingDeployment.Spec.Template.Spec.Containers = desiredDeployment.Spec.Template.Spec.Containers
	}
	if !containersEqual(existingDeployment.Spec.Template.Spec.InitContainers, desiredDeployment.Spec.Template.Spec.InitContainers) {
		changed = true
		existingDeployment.Spec.Template.Spec.InitContainers = desiredDeployment.Spec.Template.Spec.InitContainers
	}

	if changed {
		r.logger.Info("updating OLS deployment", "name", existingDeployment.Name)
		if err := r.Update(ctx, existingDeployment); err != nil {
			return err
		}
	} else {
		r.logger.Info("OLS deployment reconciliation skipped", "deployment", existingDeployment.Name, "olsconfig hash", existingDeployment.Annotations[OLSConfigHashKey])
	}

	return nil
}

func (r *OLSConfigReconciler) telemetryEnabled() (bool, error) {
	// Telemetry enablement is determined by the presence of the telemetry pull secret
	// the presence of the field '.auths."cloud.openshift.com"' indicates that telemetry is enabled
	// use this command to check in an Openshift cluster
	// oc get secret/pull-secret -n openshift-config --template='{{index .data ".dockerconfigjson" | base64decode}}' | jq '.auths."cloud.openshift.com"'
	// #nosec G101
	const pullSecretName = "pull-secret"
	// #nosec G101
	const pullSecretNamespace = "openshift-config"

	pullSecret := &corev1.Secret{}
	err := r.Get(context.Background(), client.ObjectKey{Namespace: pullSecretNamespace, Name: pullSecretName}, pullSecret)

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

func (r *OLSConfigReconciler) dataCollectorEnabled(cr *olsv1alpha1.OLSConfig) (bool, error) {
	// data collector is enabled in OLS configuration
	configEnabled := !cr.Spec.OLSConfig.UserDataCollection.FeedbackDisabled || !cr.Spec.OLSConfig.UserDataCollection.TranscriptsDisabled
	telemetryEnabled, err := r.telemetryEnabled()
	if err != nil {
		return false, err
	}
	return configEnabled && telemetryEnabled, nil
}
