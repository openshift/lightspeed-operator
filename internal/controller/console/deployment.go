package console

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// GenerateConsoleUIDeployment generates the Console UI deployment object.
func GenerateConsoleUIDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	labels := GenerateConsoleUILabels()
	resources := utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Resources,
		utils.DefaultConsolePluginResourceRequirements(),
	)

	return utils.GenerateConsolePluginDeployment(r, cr, utils.ConsolePluginDeploymentOptions{
		Name:                utils.ConsoleUIDeploymentName,
		Labels:              labels,
		SelectorLabels:      labels,
		ServiceAccountName:  utils.ConsoleUIServiceAccountName,
		ContainerName:       utils.ConsoleUIContainerName,
		Image:               r.GetConsoleUIImage(),
		Port:                utils.ConsoleUIHTTPSPort,
		PortName:            "https",
		CertVolumeName:      "lightspeed-console-plugin-cert",
		CertSecretName:      utils.ConsoleUIServiceCertSecretName,
		NginxVolumeName:     "nginx-config",
		NginxConfigMapName:  utils.ConsoleUIConfigMapName,
		NginxTempVolumeName: "nginx-temp",
		Resources:           resources,
		Env: append(utils.GetProxyEnvVars(), corev1.EnvVar{
			Name:  "OCP_VERSION",
			Value: r.GetOpenShiftMajor() + "." + r.GetOpenshiftMinor(),
		}),
		DeploymentConfig: cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer,
	})
}
