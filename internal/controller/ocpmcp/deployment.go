package ocpmcp

import (
	"context"
	"fmt"
	"path"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func getResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	// Defaults follow OpenShift conventions: requests only (OLS-3397).
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.MCPServerContainer.Resources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Claims: []corev1.ResourceClaim{},
		},
	)
}

func getSecretResourceVersion(r reconciler.Reconciler, ctx context.Context, secretName string) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: r.GetNamespace()}, secret)
	if err != nil {
		return "", fmt.Errorf("%s: %w", utils.ErrGetOpenShiftMCPServerTLSSecret, err)
	}
	return secret.ResourceVersion, nil
}

// GenerateDeployment generates the standalone openshift-mcp-server Deployment with
// service-ca TLS (--tls-cert/--tls-key), HTTPS probes on /healthz, and user-configurable replicas.
func GenerateDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	revisionHistoryLimit := int32(1)
	runAsNonRoot := true

	configMapResourceVersion, err := utils.GetConfigMapResourceVersion(r, ctx, utils.OpenShiftMCPServerConfigCmName)
	if err != nil {
		return nil, err
	}
	tlsSecretResourceVersion, err := getSecretResourceVersion(r, ctx, utils.OpenShiftMCPServerCertsSecretName)
	if err != nil {
		return nil, err
	}

	configVolume, configMount := utils.GetOpenShiftMCPServerConfigVolumeAndMount()
	tlsVolumeDefaultMode := utils.VolumeRestrictedMode
	httpsPort := intstr.FromInt32(utils.OpenShiftMCPServerHTTPSPort)
	configPath := utils.GetOpenShiftMCPServerConfigPath()
	tlsCertPath := path.Join(utils.OpenShiftMCPServerTLSMountPath, "tls.crt")
	tlsKeyPath := path.Join(utils.OpenShiftMCPServerTLSMountPath, "tls.key")

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.OpenShiftMCPServerDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
			Annotations: map[string]string{
				utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation: configMapResourceVersion,
				utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation: tlsSecretResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels(),
			},
			RevisionHistoryLimit: &revisionHistoryLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selectorLabels(),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: utils.OpenShiftMCPServerServiceAccountName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            utils.OpenShiftMCPServerContainerName,
							Image:           r.GetOpenShiftMCPServerImage(),
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: utils.RestrictedContainerSecurityContext(),
							Command: []string{
								"/openshift-mcp-server",
								"--config", configPath,
								"--port", fmt.Sprintf("%d", utils.OpenShiftMCPServerHTTPSPort),
								"--tls-cert=" + tlsCertPath,
								"--tls-key=" + tlsKeyPath,
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "https",
									ContainerPort: utils.OpenShiftMCPServerHTTPSPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: *getResources(cr),
							VolumeMounts: []corev1.VolumeMount{
								configMount,
								{
									Name:      utils.OpenShiftMCPServerTLSVolumeName,
									MountPath: utils.OpenShiftMCPServerTLSMountPath,
									ReadOnly:  true,
								},
								{
									Name:      utils.TmpVolumeName,
									MountPath: utils.TmpVolumeMountPath,
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   httpsPort,
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    30,
								FailureThreshold: 3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/healthz",
										Port:   httpsPort,
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    10,
								FailureThreshold: 3,
							},
						},
					},
					Volumes: []corev1.Volume{
						configVolume,
						{
							Name: utils.OpenShiftMCPServerTLSVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  utils.OpenShiftMCPServerCertsSecretName,
									DefaultMode: &tlsVolumeDefaultMode,
								},
							},
						},
						{
							Name: utils.TmpVolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	utils.ApplyPodDeploymentConfig(deployment, cr.Spec.OLSConfig.DeploymentConfig.MCPServerContainer, true)

	if err := controllerutil.SetControllerReference(cr, deployment, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetOpenShiftMCPServerDeploymentOwnerReference, err)
	}

	return deployment, nil
}

// UpdateDeployment updates the MCP Deployment when the pod spec, config ConfigMap, or TLS Secret changes.
func UpdateDeployment(r reconciler.Reconciler, ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	utils.SetDefaults_Deployment(desiredDeployment)
	changed := !utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec, false)

	if existingDeployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation] !=
		desiredDeployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation] {
		changed = true
	}
	if existingDeployment.Annotations[utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation] !=
		desiredDeployment.Annotations[utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation] {
		changed = true
	}

	if !changed {
		return nil
	}

	existingDeployment.Spec = desiredDeployment.Spec
	if existingDeployment.Annotations == nil {
		existingDeployment.Annotations = make(map[string]string)
	}
	existingDeployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation] =
		desiredDeployment.Annotations[utils.OpenShiftMCPServerConfigMapResourceVersionAnnotation]
	existingDeployment.Annotations[utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation] =
		desiredDeployment.Annotations[utils.OpenShiftMCPServerTLSSecretResourceVersionAnnotation]

	// Stamp force-reload on the desired template and persist in a single Update so
	// Restart is not required to mutate a shared in-memory object.
	if existingDeployment.Spec.Template.Annotations == nil {
		existingDeployment.Spec.Template.Annotations = make(map[string]string)
	}
	existingDeployment.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("updating openshift-mcp-server deployment", "name", existingDeployment.Name)
	if err := r.Update(ctx, existingDeployment); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateOpenShiftMCPServerDeployment, err)
	}
	return nil
}

// Restart triggers a rolling restart of the MCP server Deployment.
// Always re-fetches from the API so callers (TLS watcher) do not depend on a
// shared in-memory Deployment. NotFound is a no-op so races during introspection
// disable (Remove deletes the Deployment) do not fail reconciliation.
// The optional deployment argument is accepted for watcher signature compatibility
// but is not used for the update payload.
func Restart(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	_ = deployment

	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.OpenShiftMCPServerDeploymentName, Namespace: r.GetNamespace()}, dep)
	if err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("openshift-mcp-server deployment not found, skipping restart",
				"deployment", utils.OpenShiftMCPServerDeploymentName)
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrUpdateOpenShiftMCPServerDeployment, err)
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("triggering openshift-mcp-server rolling restart", "deployment", dep.Name)
	if err := r.Update(ctx, dep); err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("openshift-mcp-server deployment not found during restart, skipping",
				"deployment", dep.Name)
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrUpdateOpenShiftMCPServerDeployment, err)
	}
	return nil
}
