package rhokp

import (
	"context"
	"fmt"
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
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.RHOKPContainer.Resources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("2"),
				corev1.ResourceMemory:           resource.MustParse("2Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse(utils.RHOKPSolrDataSizeLimitDefault),
			},
			Claims: []corev1.ResourceClaim{},
		},
	)
}

func getSecretResourceVersion(r reconciler.Reconciler, ctx context.Context, secretName string) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: r.GetNamespace()}, secret)
	if err != nil {
		return "", fmt.Errorf("%s: %w", utils.ErrGetRHOKPTLSSecret, err)
	}
	return secret.ResourceVersion, nil
}

// GenerateDeployment generates the standalone RHOKP Deployment with service-ca TLS
// mounted for Apache httpd, an EmptyDir for Solr data, and HTTPS probes.
func GenerateDeployment(r reconciler.Reconciler, ctx context.Context, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	revisionHistoryLimit := int32(1)
	runAsNonRoot := true

	tlsSecretResourceVersion, err := getSecretResourceVersion(r, ctx, utils.RHOKPCertsSecretName)
	if err != nil {
		return nil, err
	}

	tlsVolumeDefaultMode := utils.VolumeRestrictedMode
	httpsPort := intstr.FromInt32(utils.RHOOKPImageHTTPSPort)

	// Use ephemeral-storage from resolved resources (CRD override or default).
	resources := getResources(cr)
	solrDataSizeLimit := resource.MustParse(utils.RHOKPSolrDataSizeLimitDefault)
	if es, ok := resources.Requests[corev1.ResourceEphemeralStorage]; ok {
		solrDataSizeLimit = es
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RHOKPDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    selectorLabels(),
			Annotations: map[string]string{
				utils.RHOKPTLSSecretResourceVersionAnnotation: tlsSecretResourceVersion,
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
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:            utils.RHOOKPContainerName,
							Image:           r.GetRHOOKPImage(),
							ImagePullPolicy: corev1.PullIfNotPresent,
							SecurityContext: utils.RHOOKPContainerSecurityContext(),
							Env:             generateRHOOKPEnv(),
							Ports: []corev1.ContainerPort{
								{
									Name:          "https",
									ContainerPort: utils.RHOOKPImageHTTPSPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Resources: *resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      utils.RHOKPTLSVolumeName,
									MountPath: utils.RHOKPTLSMountPath,
									ReadOnly:  true,
								},
								{
									Name:      utils.RHOKPSolrDataVolumeName,
									MountPath: utils.RHOKPSolrDataMountPath,
								},
							},
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   utils.RHOOKPReadinessHTTPPath,
										Port:   httpsPort,
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								InitialDelaySeconds: utils.RHOOKPStartupProbeInitialDelaySeconds,
								PeriodSeconds:       utils.RHOOKPStartupProbePeriodSeconds,
								FailureThreshold:    utils.RHOOKPStartupProbeFailureThreshold,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   utils.RHOOKPReadinessHTTPPath,
										Port:   httpsPort,
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    utils.RHOOKPProbePeriodSeconds,
								TimeoutSeconds:   utils.RHOOKPProbeTimeoutSeconds,
								FailureThreshold: utils.RHOKPReadinessProbeFailureThreshold,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   utils.RHOOKPReadinessHTTPPath,
										Port:   httpsPort,
										Scheme: corev1.URISchemeHTTPS,
									},
								},
								PeriodSeconds:    utils.RHOOKPProbePeriodSeconds,
								TimeoutSeconds:   utils.RHOOKPProbeTimeoutSeconds,
								FailureThreshold: utils.RHOKPProbeFailureThreshold,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: utils.RHOKPTLSVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  utils.RHOKPCertsSecretName,
									DefaultMode: &tlsVolumeDefaultMode,
									Items: []corev1.KeyToPath{
										{Key: "tls.crt", Path: "localhost.crt"},
										{Key: "tls.key", Path: "localhost.key"},
									},
								},
							},
						},
						{
							Name: utils.RHOKPSolrDataVolumeName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{
									SizeLimit: &solrDataSizeLimit,
								},
							},
						},
					},
				},
			},
		},
	}

	utils.ApplyPodDeploymentConfig(deployment, cr.Spec.OLSConfig.DeploymentConfig.RHOKPContainer, false)

	if err := controllerutil.SetControllerReference(cr, deployment, r.GetScheme()); err != nil {
		return nil, fmt.Errorf("%s: %w", utils.ErrSetRHOKPDeploymentOwnerReference, err)
	}

	return deployment, nil
}

// UpdateDeployment updates the RHOKP Deployment when the pod spec or TLS Secret changes.
func UpdateDeployment(r reconciler.Reconciler, ctx context.Context, existingDeployment, desiredDeployment *appsv1.Deployment) error {
	utils.SetDefaults_Deployment(desiredDeployment)
	changed := !utils.DeploymentSpecEqual(&existingDeployment.Spec, &desiredDeployment.Spec, false)

	if existingDeployment.Annotations[utils.RHOKPTLSSecretResourceVersionAnnotation] !=
		desiredDeployment.Annotations[utils.RHOKPTLSSecretResourceVersionAnnotation] {
		changed = true
	}

	if !changed {
		return nil
	}

	existingDeployment.Spec = desiredDeployment.Spec
	if existingDeployment.Annotations == nil {
		existingDeployment.Annotations = make(map[string]string)
	}
	existingDeployment.Annotations[utils.RHOKPTLSSecretResourceVersionAnnotation] =
		desiredDeployment.Annotations[utils.RHOKPTLSSecretResourceVersionAnnotation]

	if existingDeployment.Spec.Template.Annotations == nil {
		existingDeployment.Spec.Template.Annotations = make(map[string]string)
	}
	existingDeployment.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("updating RHOKP deployment", "name", existingDeployment.Name)
	if err := r.Update(ctx, existingDeployment); err != nil {
		return fmt.Errorf("%s: %w", utils.ErrUpdateRHOKPDeployment, err)
	}
	return nil
}

// Restart triggers a rolling restart of the RHOKP Deployment.
// Re-fetches from the API so callers (TLS watcher) do not depend on a shared
// in-memory Deployment. NotFound is a no-op so races during byokRAGOnly enable
// (Remove deletes the Deployment) do not fail reconciliation.
func Restart(r reconciler.Reconciler, ctx context.Context, deployment ...*appsv1.Deployment) error {
	_ = deployment

	dep := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: utils.RHOKPDeploymentName, Namespace: r.GetNamespace()}, dep)
	if err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("RHOKP deployment not found, skipping restart",
				"deployment", utils.RHOKPDeploymentName)
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrUpdateRHOKPDeployment, err)
	}

	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = make(map[string]string)
	}
	dep.Spec.Template.Annotations[utils.ForceReloadAnnotationKey] = time.Now().Format(time.RFC3339Nano)

	r.GetLogger().Info("triggering RHOKP rolling restart", "deployment", dep.Name)
	if err := r.Update(ctx, dep); err != nil {
		if errors.IsNotFound(err) {
			r.GetLogger().Info("RHOKP deployment not found during restart, skipping",
				"deployment", dep.Name)
			return nil
		}
		return fmt.Errorf("%s: %w", utils.ErrUpdateRHOKPDeployment, err)
	}
	return nil
}

// generateRHOOKPEnv returns environment variables for the RHOKP container.
func generateRHOOKPEnv() []corev1.EnvVar {
	optional := true
	return []corev1.EnvVar{
		{
			Name: "ACCESS_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: utils.RHOOKPAccessKeySecretName,
					},
					Key:      utils.RHOOKPAccessKeySecretKey,
					Optional: &optional,
				},
			},
		},
	}
}
