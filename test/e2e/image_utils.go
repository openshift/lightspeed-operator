package e2e

import (
	"context"
	"fmt"
	"io"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	imagev1 "github.com/openshift/api/image/v1"
	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	routev1 "github.com/openshift/api/route/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"

	"github.com/openshift/lightspeed-operator/internal/controller/utils"
)

func CopyImageToRegistry(
	ctx context.Context,
	srcRef string,
	destRegistry, destNamespace, destImage string,
	srcUser, srcPass, dstUser, dstPass string,
	insecureSrc, insecureDst bool,
	reportWriter io.Writer,
	timeout time.Duration,
) (string, error) {
	if destRegistry == "" || destNamespace == "" || destImage == "" {
		return "", fmt.Errorf("destRegistry, destNamespace and destImage must be provided")
	}
	destRefStr := fmt.Sprintf("docker://%s/%s/%s", destRegistry, destNamespace, destImage)

	src, err := alltransports.ParseImageName(srcRef)
	if err != nil {
		return "", fmt.Errorf("parsing source reference %q: %w", srcRef, err)
	}
	dst, err := alltransports.ParseImageName(destRefStr)
	if err != nil {
		return "", fmt.Errorf("parsing destination reference %q: %w", destRefStr, err)
	}

	var srcSys types.SystemContext
	if srcUser != "" || srcPass != "" {
		srcSys.DockerAuthConfig = &types.DockerAuthConfig{
			Username: srcUser,
			Password: srcPass,
		}
	}
	if insecureSrc {
		srcSys.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
	}

	var dstSys types.SystemContext
	if dstUser != "" || dstPass != "" {
		dstSys.DockerAuthConfig = &types.DockerAuthConfig{
			Username: dstUser,
			Password: dstPass,
		}
	}
	if insecureDst {
		dstSys.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
	}

	policyJSON := []byte(`{"default":[{"type":"insecureAcceptAnything"}]}`)
	policy, err := signature.NewPolicyFromBytes(policyJSON)
	if err != nil {
		return "", fmt.Errorf("creating signature policy: %w", err)
	}
	policyCtx, err := signature.NewPolicyContext(policy)
	if err != nil {
		return "", fmt.Errorf("creating signature policy context: %w", err)
	}
	defer func() { _ = policyCtx.Destroy() }()

	opts := &copy.Options{
		SourceCtx:      &srcSys,
		DestinationCtx: &dstSys,
		ReportWriter:   reportWriter,
	}

	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var manifestBytes []byte
	if manifestBytes, err = copy.Image(ctx, policyCtx, dst, src, opts); err != nil {
		return "", fmt.Errorf("copy.Image failed: %w", err)
	}

	digest, err := manifest.Digest(manifestBytes)
	if err != nil {
		return "", fmt.Errorf("compute digest: %w", err)
	}

	return digest.String(), nil
}

func AddImageStreamImport(client *Client, namespace, tagName, fromRef string) error {
	isName := utils.ImageStreamNameFor(fromRef)
	isi := &imagev1.ImageStreamImport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      isName,
			Namespace: utils.OLSNamespaceDefault,
		},
		Spec: imagev1.ImageStreamImportSpec{
			Import: true,
			Images: []imagev1.ImageImportSpec{
				{
					From: corev1.ObjectReference{
						Kind: "DockerImage",
						Name: fromRef,
					},
					To: &corev1.LocalObjectReference{
						Name: tagName,
					},
				},
			},
		},
	}
	if err := client.Create(isi); err != nil {
		return fmt.Errorf("failed to create ImageStreamImport %s/%s: %w", namespace, isName, err)
	}
	return nil
}

func GetInternalImageRegistryRoute(client *Client) (string, error) {
	const (
		defaultRouteName       = "default-route"
		imageRegistryNamespace = "openshift-image-registry"
	)
	route := routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      defaultRouteName,
			Namespace: imageRegistryNamespace,
		},
	}
	if err := client.WaitForObjectCreated(&route); err != nil {
		return "", fmt.Errorf("error waiting for the internal registry route creation: %w", err)
	}
	if err := client.Get(&route); err != nil {
		return "", fmt.Errorf("failed to get the internal image registry route %s/%s: %w", imageRegistryNamespace, defaultRouteName, err)
	}
	return route.Spec.Host, nil
}

func EnableInternalImageRegistryRoute(client *Client) error {
	cfg := imageregistryv1.Config{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
	if err := client.Get(&cfg); err != nil {
		return fmt.Errorf("get imageregistry config/cluster: %w", err)
	}

	if err := client.Update(&cfg, func(obj ctrlclient.Object) error {
		imgRegistryConfig := obj.(*imageregistryv1.Config)
		imgRegistryConfig.Spec.DefaultRoute = true
		return nil
	}); err != nil {
		return fmt.Errorf("update imageregistry config/cluster: %w", err)
	}

	return nil
}

func GetBuilderToken(client *Client, namespace, saName string) (string, error) {
	ctx := context.Background()
	expSeconds := int64(3600)

	issuer, err := GetOAuthIssuer(client)
	if err != nil {
		return "", err
	}
	audience := "https://kubernetes.default.svc"
	if issuer != "" {
		audience = issuer
	}

	tr := &authv1.TokenRequest{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TokenRequest",
			APIVersion: "authentication.k8s.io/v1",
		},
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{audience},
			ExpirationSeconds: &expSeconds,
		},
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      saName,
		},
	}

	if err := client.kClient.SubResource("token").Create(ctx, sa, tr); err != nil {
		return "", fmt.Errorf("creating token subresource: %w", err)
	}
	return tr.Status.Token, nil
}

func AddImageBuilderRole(client *Client, namespace, saName string) error {
	const rbName = "allow-builder-image-build"

	newRB := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rbName,
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:image-builder",
		},
	}
	if err := client.Create(newRB); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating RoleBinding %s/%s: %w", namespace, rbName, err)
		}
	}
	return nil
}

func GetOAuthIssuer(client *Client) (string, error) {
	auth := &configv1.Authentication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}

	if err := client.Get(auth); err != nil {
		return "", fmt.Errorf("getting Authentication/cluster: %w", err)
	}

	return auth.Spec.ServiceAccountIssuer, nil
}
