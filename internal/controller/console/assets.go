package console

import (
	consolev1 "github.com/openshift/api/console/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func GenerateConsoleUILabels() map[string]string {
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

func GenerateConsoleUIConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
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
			Name:      utils.ConsoleUIConfigMapName,
			Namespace: r.GetNamespace(),
			Labels:    GenerateConsoleUILabels(),
		},
		Data: map[string]string{
			"nginx.conf": nginxConfig,
		},
	}
	if err := controllerutil.SetControllerReference(cr, cm, r.GetScheme()); err != nil {
		return nil, err
	}

	return cm, nil
}

func GenerateConsoleUIService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ConsoleUIServiceName,
			Namespace: r.GetNamespace(),
			Labels:    GenerateConsoleUILabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.ConsoleUIServiceCertSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       utils.ConsoleUIHTTPSPort,
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.Parse("https"),
				},
			},
			Selector: GenerateConsoleUILabels(),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GenerateConsoleUIDeployment(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*appsv1.Deployment, error) {
	const certVolumeName = "lightspeed-console-plugin-cert"
	val_true := true
	volumeDefaultMode := int32(420)
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

func GenerateConsoleUIPlugin(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*consolev1.ConsolePlugin, error) {
	plugin := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.ConsoleUIPluginName,
			Labels: GenerateConsoleUILabels(),
		},
		Spec: consolev1.ConsolePluginSpec{
			Backend: consolev1.ConsolePluginBackend{
				Service: &consolev1.ConsolePluginService{
					Name:      utils.ConsoleUIServiceName,
					Namespace: r.GetNamespace(),
					Port:      utils.ConsoleUIHTTPSPort,
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
					Alias:         utils.ConsoleProxyAlias,
					Authorization: consolev1.UserToken,
					Endpoint: consolev1.ConsolePluginProxyEndpoint{
						Service: &consolev1.ConsolePluginProxyServiceConfig{
							Name:      utils.OLSAppServerServiceName,
							Namespace: r.GetNamespace(),
							Port:      utils.OLSAppServerServicePort,
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

	if err := controllerutil.SetControllerReference(cr, plugin, r.GetScheme()); err != nil {
		return nil, err
	}

	return plugin, nil
}

func GenerateConsoleUINetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ConsoleUINetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    GenerateConsoleUILabels(),
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
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.ConsoleUIHTTPSPort)}[0],
						},
					},
				},
			},
			PodSelector: metav1.LabelSelector{
				MatchLabels: GenerateConsoleUILabels(),
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &np, r.GetScheme()); err != nil {
		return nil, err
	}
	return &np, nil

}
