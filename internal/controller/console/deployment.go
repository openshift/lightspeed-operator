package console

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// getConsoleUIResources returns the resource requirements for the console UI container.
func getConsoleUIResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	return utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Resources,
		&corev1.ResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("50Mi")},
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("100Mi")},
			Claims:   []corev1.ResourceClaim{},
		},
	)
}

// GenerateConsoleUIDeployment generates the Console UI deployment object.
func GenerateConsoleUIDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	const certVolumeName = "lightspeed-console-plugin-cert"
	val_true := true
	volumeDefaultMode := utils.VolumeDefaultMode
	resources := getConsoleUIResources(cr)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ConsoleUIDeploymentName,
			Namespace: r.GetNamespace(),
			Labels:    GenerateConsoleUILabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: GenerateConsoleUILabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: GenerateConsoleUILabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "lightspeed-console-plugin",
							Image: r.GetConsoleUIImage(),
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: utils.ConsoleUIHTTPSPort,
									Name:          "https",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								ReadOnlyRootFilesystem:   &[]bool{true}[0],
							},
							ImagePullPolicy: corev1.PullAlways,
							Env:             utils.GetProxyEnvVars(),
							Resources:       *resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      certVolumeName,
									MountPath: "/var/cert",
									ReadOnly:  true,
								},
								{
									Name:      "nginx-config",
									MountPath: "/etc/nginx/nginx.conf",
									SubPath:   "nginx.conf",
									ReadOnly:  true,
								},
								{
									Name:      "nginx-temp",
									MountPath: "/tmp/nginx",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: certVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  utils.ConsoleUIServiceCertSecretName,
									DefaultMode: &volumeDefaultMode,
								},
							},
						},
						{
							Name: "nginx-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: utils.ConsoleUIConfigMapName,
									},
									DefaultMode: &volumeDefaultMode,
								},
							},
						},
						{
							Name: "nginx-temp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &val_true,
						SeccompProfile: &corev1.SeccompProfile{
							Type: "RuntimeDefault",
						},
					},
				},
			},
		},
	}

	if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.NodeSelector != nil {
		deployment.Spec.Template.Spec.NodeSelector = cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.NodeSelector
	}
	if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Tolerations != nil {
		deployment.Spec.Template.Spec.Tolerations = cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Tolerations
	}

	if err := controllerutil.SetControllerReference(cr, deployment, r.GetScheme()); err != nil {
		return nil, err
	}

	return deployment, nil
}
