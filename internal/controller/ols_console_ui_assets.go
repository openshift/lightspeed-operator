package controller

import (
	"fmt"

	consolev1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

func getConsoleUIResources(cr *olsv1alpha1.OLSConfig) *corev1.ResourceRequirements {
	if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Resources != nil {
		return cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Resources
	}
	// default resources.
	defaultResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("50Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("100Mi")},
		Claims:   []corev1.ResourceClaim{},
	}

	return defaultResources
}

func getHideIconEnvVar(cr *olsv1alpha1.OLSConfig) corev1.EnvVar {
	// The CRD has a default of false, so we can use the value directly
	// If the field is not set, it will default to false
	hideIcon := cr.Spec.OLSConfig.HideIcon

	return corev1.EnvVar{
		Name:  "HIDE_ICON",
		Value: fmt.Sprintf("%t", hideIcon),
	}
}

func (r *OLSConfigReconciler) generateConsoleUIConfigMap(cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	nginxConfig := `
			pid       /tmp/nginx/nginx.pid;
			error_log /dev/stdout info;
			events {}
			http {
				client_body_temp_path /tmp/nginx/client_body;
				proxy_temp_path       /tmp/nginx/proxy;
				fastcgi_temp_path     /tmp/nginx/fastcgi;
				uwsgi_temp_path       /tmp/nginx/uwsgi;
				scgi_temp_path        /tmp/nginx/scgi;
				access_log            /dev/stdout;
				include               /etc/nginx/mime.types;
				default_type          application/octet-stream;
				keepalive_timeout     65;
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
	const certVolumeName = "lightspeed-console-plugin-cert"
	val_true := true
	volumeDefaultMode := int32(420)
	resources := getConsoleUIResources(cr)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUIDeploymentName,
			Namespace: r.Options.Namespace,
			Labels:    generateConsoleUILabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.Replicas,
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
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &[]bool{false}[0],
								ReadOnlyRootFilesystem:   &[]bool{true}[0],
							},
							ImagePullPolicy: corev1.PullAlways,
							Env:             append(getProxyEnvVars(), getHideIconEnvVar(cr)),
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

	// Conditionally add the CA certificate if provided in the CR
	if cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.CAcertificate != "" {
		plugin.Spec.Proxy[0].CACertificate = cr.Spec.OLSConfig.DeploymentConfig.ConsoleContainer.CAcertificate
	}

	if err := controllerutil.SetControllerReference(cr, plugin, r.Scheme); err != nil {
		return nil, err
	}

	return plugin, nil
}

func (r *OLSConfigReconciler) generateConsoleUINetworkPolicy(cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConsoleUINetworkPolicyName,
			Namespace: r.Options.Namespace,
			Labels:    generateConsoleUILabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": "openshift-console",
								},
							},
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "console",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(ConsoleUIHTTPSPort)}[0],
						},
					},
				},
			},
			PodSelector: metav1.LabelSelector{
				MatchLabels: generateConsoleUILabels(),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &np, r.Scheme); err != nil {
		return nil, err
	}
	return &np, nil

}
