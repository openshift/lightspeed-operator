package console

import (
	"fmt"

	consolev1 "github.com/openshift/api/console/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

	// Conditionally add the CA certificate if user provides custom TLS
	// If TLSConfig.KeyCertSecretRef is set, try to fetch the CA cert from that Secret
	if cr.Spec.OLSConfig.TLSConfig != nil && cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name != "" {
		caCert, err := utils.GetCAFromSecret(r, r.GetNamespace(), cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get CA certificate from Secret %s: %w", cr.Spec.OLSConfig.TLSConfig.KeyCertSecretRef.Name, err)
		}
		// Only set CA certificate if one was found in the secret
		if caCert != "" {
			plugin.Spec.Proxy[0].CACertificate = caCert
		}
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
