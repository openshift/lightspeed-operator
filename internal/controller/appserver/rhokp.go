package appserver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// rhokpStartupScript remaps RHOKP Apache HTTP/HTTPS listen ports before mel start so MCP (8080)
// and the app server (8443) can keep the stock ports in the shared pod network namespace.
// ssl.conf must remain present — mel's httpd pre-init reads it for TLS cert setup.
func rhokpStartupScript() string {
	return fmt.Sprintf(
		`sed -i 's/^Listen %d/Listen %d/' %s && `+
			`sed -i 's/^Listen 0.0.0.0:%d https/Listen 0.0.0.0:%d https/' %s && `+
			`sed -i 's/_default_:%d/_default_:%d/' %s && `+
			`exec %s %s`,
		utils.RHOOKPImageHTTPPort, utils.RHOOKPHTTPPort, utils.RHOOKPHTTPDConfPath,
		utils.RHOOKPImageHTTPSPort, utils.RHOOKPHTTPSPort, utils.RHOOKPHTTPDSSLConfPath,
		utils.RHOOKPImageHTTPSPort, utils.RHOOKPHTTPSPort, utils.RHOOKPHTTPDSSLConfPath,
		utils.RHOOKPContainerEntrypoint,
		utils.RHOOKPMainCommand,
	)
}

func rhokpContainerCommand() []string {
	return []string{"/bin/sh", "-c"}
}

func rhokpContainerArgs() []string {
	return []string{rhokpStartupScript()}
}

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
