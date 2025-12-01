package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	"github.com/openshift/lightspeed-operator/internal/controller/reconciler"
	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func GeneratePostgresService(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Service, error) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresServiceName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
			Annotations: map[string]string{
				utils.ServingCertSecretAnnotationKey: utils.PostgresCertsSecretName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       utils.PostgresServicePort,
					Protocol:   corev1.ProtocolTCP,
					Name:       "server",
					TargetPort: intstr.Parse("server"),
				},
			},
			Selector: utils.GeneratePostgresSelectorLabels(),
			Type:     corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &service, r.GetScheme()); err != nil {
		return nil, err
	}

	return &service, nil
}

func GeneratePostgresSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	randomPassword := make([]byte, 12)
	_, err := rand.Read(randomPassword)
	if err != nil {
		return nil, fmt.Errorf("error generating random password: %w", err)
	}
	// Encode the password to base64
	encodedPassword := base64.StdEncoding.EncodeToString(randomPassword)
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresSecretName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		Data: map[string][]byte{
			utils.PostgresSecretKeyName: []byte(encodedPassword),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return &secret, nil
}

func GeneratePostgresBootstrapSecret(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.Secret, error) {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresBootstrapSecretName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		StringData: map[string]string{
			utils.PostgresExtensionScript: string(utils.PostgresBootStrapScriptContent),
		},
	}

	if err := controllerutil.SetControllerReference(cr, &secret, r.GetScheme()); err != nil {
		return nil, err
	}

	return &secret, nil
}

func GeneratePostgresConfigMap(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.ConfigMap, error) {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresConfigMap,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		Data: map[string]string{
			utils.PostgresConfig: utils.PostgresConfigMapContent,
		},
	}

	if err := controllerutil.SetControllerReference(cr, &configMap, r.GetScheme()); err != nil {
		return nil, err
	}

	return &configMap, nil
}

func GeneratePostgresNetworkPolicy(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*networkingv1.NetworkPolicy, error) {
	np := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresNetworkPolicyName,
			Namespace: r.GetNamespace(),
			Labels:    utils.GeneratePostgresSelectorLabels(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: utils.GenerateAppServerSelectorLabels(),
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &[]intstr.IntOrString{intstr.FromInt(utils.PostgresServicePort)}[0],
						},
					},
				},
			},
			PodSelector: metav1.LabelSelector{
				MatchLabels: utils.GeneratePostgresSelectorLabels(),
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

func storageDefaults(r reconciler.Reconciler, s *olsv1alpha1.Storage) error {
	if s.Size.IsZero() {
		s.Size = resource.MustParse(utils.PostgresDefaultPVCSize)
	}
	if s.Class == "" {
		var scList storagev1.StorageClassList
		ctx := context.Background()
		if err := r.List(ctx, &scList); err == nil {
			for _, sc := range scList.Items {
				if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
					s.Class = sc.Name
				}
			}
		}
		if s.Class == "" {
			return fmt.Errorf("no storage class specified and no default storage class configured")
		}
	}
	return nil
}

func GeneratePostgresPVC(r reconciler.Reconciler, cr *olsv1alpha1.OLSConfig) (*corev1.PersistentVolumeClaim, error) {

	storage := cr.Spec.OLSConfig.Storage
	if err := storageDefaults(r, storage); err != nil {
		return nil, err
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PostgresPVCName,
			Namespace: r.GetNamespace(),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.PersistentVolumeAccessMode("ReadWriteOnce"),
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storage.Size,
				},
			},
			StorageClassName: &storage.Class,
		},
	}

	if err := controllerutil.SetControllerReference(cr, pvc, r.GetScheme()); err != nil {
		return nil, err
	}
	return pvc, nil
}
