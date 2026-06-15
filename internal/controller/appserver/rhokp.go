package appserver

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

// rhokpStartupScript disables the stock RHOKP Apache HTTPS listener on 8443 (conflicts with the
// app server in the shared pod network) then runs the image entrypoint.
func rhokpStartupScript() string {
	return fmt.Sprintf(
		"sed -i 's/^Listen 0.0.0.0:8443/# disabled for Lightspeed sidecar: &/' %s && exec %s %s",
		utils.RHOOKPHTTPDSSLConfPath,
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
