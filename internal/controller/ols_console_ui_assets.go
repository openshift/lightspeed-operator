package controller

import (
	consolev1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

func generateConsoleUILabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/component":  "console-plugin",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
		"app.kubernetes.io/name":       "lightspeed-console-plugin",
		"app.kubernetes.io/part-of":    "openshift-lightspeed",
	}
}

func (r *OLSConfigReconciler) generateConsoleUIConfigMap(cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	nginxConfig := `
			error_log /dev/stdout info;
			events {}
			http {
				access_log         /dev/stdout;
				include            /etc/nginx/mime.types;
				default_type       application/octet-stream;
				keepalive_timeout  65;
				server {
					listen              9443 ssl;
					listen              [::]:9443 ssl;
					ssl_certificate     /var/cert/tls.crt;
					ssl_certificate_key /var/cert/tls.key;
					root                /usr/share/nginx/html;
				}
			}`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIConfigMapName,
			Namespace: r.Options.Namespace,
			Labels:    generateConsoleUILabels(),
		},
		Data: map[string]string{
			"nginx.conf": nginxConfig,
		},
	}
	if err := controllerutil.SetControllerReference(cr, cm, r.Scheme); err != nil {
		return nil, err
	}

	return cm, nil
}

func (r *OLSConfigReconciler) generateConsoleUIService(cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIServiceName,
			Namespace: r.Options.Namespace,
			Labels:    generateConsoleUILabels(),
			Annotations: map[string]string{
				ServingCertSecretAnnotationKey: ConsoleUIServiceCertSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       ConsoleUIHTTPSPort,
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("https"),
				},
			},
			Selector: generateConsoleUILabels(),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.Scheme); err != nil {
		return nil, err
	}

	return &service, nil
}

func (r *OLSConfigReconciler) generateConsoleUIDeployment(cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	replicas := int32(2)
	val_true := true
	volumeDefaultMode := int32(420)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIDeploymentName,
			Namespace: r.Options.Namespace,
			Labels:    generateConsoleUILabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: generateConsoleUILabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: generateConsoleUILabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "lightspeed-console-plugin",
							Image: r.Options.ConsoleUIImage,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: ConsoleUIHTTPSPort,
									Name:          "https",
									Protocol:      corev1.ProtocolTCP,
								},
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Env:             getProxyEnvVars(),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("50Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("100Mi"),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "lightspeed-console-plugin-cert",
									MountPath: "/var/cert",
									ReadOnly:  true,
								},
								{
									Name:      "nginx-config",
									MountPath: "/etc/nginx/nginx.conf",
									SubPath:   "nginx.conf",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "lightspeed-console-plugin-cert",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  ConsoleUIServiceCertSecretName,
									DefaultMode: &volumeDefaultMode,
								},
							},
						},
						{
							Name: "nginx-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: ConsoleUIConfigMapName,
									},
									DefaultMode: &volumeDefaultMode,
								},
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

	if err := controllerutil.SetControllerReference(cr, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *OLSConfigReconciler) generateConsoleUIPlugin(cr *olsv1alpha1.OLSConfig) (*consolev1.ConsolePlugin, error) {
	plugin := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:   ConsoleUIPluginName,
			Labels: generateConsoleUILabels(),
		},
		Spec: consolev1.ConsolePluginSpec{
			Backend: consolev1.ConsolePluginBackend{
				Service: &consolev1.ConsolePluginService{
					Name:      ConsoleUIServiceName,
					Namespace: r.Options.Namespace,
					Port:      ConsoleUIHTTPSPort,
					BasePath:  "/",
				},
				Type: consolev1.Service,
			},
			DisplayName: "Lightspeed Console Plugin",
			I18n: consolev1.ConsolePluginI18n{
				LoadType: consolev1.Preload,
			},
			Proxy: []consolev1.ConsolePluginProxy{
				{
					Alias:         ConsoleProxyAlias,
					Authorization: consolev1.UserToken,
					Endpoint: consolev1.ConsolePluginProxyEndpoint{
						Service: &consolev1.ConsolePluginProxyServiceConfig{
							Name:      OLSAppServerServiceName,
							Namespace: r.Options.Namespace,
							Port:      OLSAppServerServicePort,
						},
						Type: consolev1.ProxyTypeService,
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, plugin, r.Scheme); err != nil {
		return nil, err
	}

	return plugin, nil
}
