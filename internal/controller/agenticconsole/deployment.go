package agenticconsole

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// GenerateAgenticConsoleUIDeployment generates the agentic console UI deployment object.
func GenerateAgenticConsoleUIDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	labels := GenerateAgenticConsoleUILabels()

	var userResources *corev1.ResourceRequirements
	var deploymentConfig olsv1alpha1.Config
	if cr.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer != nil {
		userResources = cr.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer.Resources
		deploymentConfig = *cr.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer
	}
	resources := utils.GetResourcesOrDefault(userResources, utils.DefaultConsolePluginResourceRequirements())

	return utils.GenerateConsolePluginDeployment(r, cr, utils.ConsolePluginDeploymentOptions{
		Name:                utils.AgenticConsoleUIDeploymentName,
		Labels:              labels,
		SelectorLabels:      map[string]string{"app.kubernetes.io/name": utils.AgenticConsoleUIPluginName},
		ServiceAccountName:  utils.AgenticConsoleUIServiceAccountName,
		ContainerName:       utils.AgenticConsoleUIContainerName,
		Image:               r.GetAgenticConsoleImage(),
		Port:                utils.AgenticConsoleUIHTTPSPort,
		CertVolumeName:      "cert",
		CertSecretName:      utils.AgenticConsoleUIServiceCertSecretName,
		NginxVolumeName:     "nginx-conf",
		NginxConfigMapName:  utils.AgenticConsoleUIConfigMapName,
		NginxTempVolumeName: "nginx-tmp",
		Resources:           resources,
		DeploymentConfig:    deploymentConfig,
	})
}
