package agenticconsole

import (
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

func GenerateAgenticConsoleUILabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       utils.AgenticConsoleUIPluginName,
		"app.kubernetes.io/component":  "console",
		"app.kubernetes.io/managed-by": "lightspeed-operator",
	}
}

func GenerateAgenticConsoleUIConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	nginxConfig := `pid /tmp/nginx/nginx.pid;
error_log /dev/stdout info;
events {}
http {
  client_body_temp_path /tmp/nginx/client_body;
  proxy_temp_path       /tmp/nginx/proxy;
  fastcgi_temp_path     /tmp/nginx/fastcgi;
  uwsgi_temp_path       /tmp/nginx/uwsgi;
  scgi_temp_path        /tmp/nginx/scgi;
  include               /etc/nginx/mime.types;
  default_type          application/octet-stream;
  keepalive_timeout     65;
  server {
    listen              9443 ssl;
    listen              [::]:9443 ssl;
    ssl_certificate     /var/cert/tls.crt;
    ssl_certificate_key /var/cert/tls.key;
    root                /usr/share/nginx/html;
    access_log          /dev/stdout;
  }
}
`

	return utils.GenerateConsolePluginNginxConfigMap(r, cr, utils.AgenticConsoleUIConfigMapName, GenerateAgenticConsoleUILabels(), nginxConfig)
}

func GenerateAgenticConsoleUIService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.AgenticConsoleUIServiceName,
			Namespace: r.GetNamespace(),
			Labels:    GenerateAgenticConsoleUILabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.AgenticConsoleUIServiceCertSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       utils.AgenticConsoleUIHTTPSPort,
					Name:       "https",
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt32(utils.AgenticConsoleUIHTTPSPort),
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name": utils.AgenticConsoleUIPluginName,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GenerateAgenticConsoleUIPlugin(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*consolev1.ConsolePlugin, error) {
	plugin := &consolev1.ConsolePlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:   utils.AgenticConsoleUIPluginName,
			Labels: GenerateAgenticConsoleUILabels(),
		},
		Spec: consolev1.ConsolePluginSpec{
			Backend: consolev1.ConsolePluginBackend{
				Service: &consolev1.ConsolePluginService{
					Name:      utils.AgenticConsoleUIServiceName,
					Namespace: r.GetNamespace(),
					Port:      utils.AgenticConsoleUIHTTPSPort,
					BasePath:  "/",
				},
				Type: consolev1.Service,
			},
			DisplayName: utils.AgenticConsoleUIPluginDisplayName,
			I18n: consolev1.ConsolePluginI18n{
				LoadType: consolev1.Preload,
			},
		},
	}

	if err := controllerutil.SetControllerReference(cr, plugin, r.GetScheme()); err != nil {
		return nil, err
	}

	return plugin, nil
}

func GenerateAgenticConsoleUINetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	labels := GenerateAgenticConsoleUILabels()
	return utils.GenerateConsolePluginNetworkPolicy(r, cr, utils.AgenticConsoleUINetworkPolicyName, labels, utils.AgenticConsoleUIHTTPSPort)
}

func GenerateAgenticConsoleUIServiceAccount(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ServiceAccount, error) {
	return utils.GenerateServiceAccount(r, cr, utils.AgenticConsoleUIServiceAccountName)
}
