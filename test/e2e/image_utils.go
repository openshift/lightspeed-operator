package e2e

import (
	"context"
	"fmt"
	"io"
	"time"

	imagev1 "github.com/openshift/api/image/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
