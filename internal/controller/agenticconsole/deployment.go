package agenticconsole

import (
	appsv1 "k8s.io/api/apps/v1"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// GenerateAgenticConsoleUIDeployment generates the agentic console UI deployment object.
func GenerateAgenticConsoleUIDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	labels := GenerateAgenticConsoleUILabels()
	resources := utils.GetResourcesOrDefault(
		cr.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer.Resources,
		utils.DefaultConsolePluginResourceRequirements(),
	)

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
		DeploymentConfig:    cr.Spec.OLSConfig.DeploymentConfig.AgenticConsoleContainer,
	})
}
