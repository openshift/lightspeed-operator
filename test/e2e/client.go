package e2e

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	consolev1 "github.com/openshift/api/console/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

const (
	// DefaultTimeout is the default timeout for client operations
	DefaultClientTimeout = 30 * time.Second
	// DefaultPollInterval is the default interval for polling
	DefaultPollInterval = 5 * time.Second
	// DefaultPollTimeout is the default timeout for polling
	DefaultPollTimeout = 10 * time.Minute
)

type ClientOptions struct {
	conditionCheckTimeout time.Duration
}

type Client struct {
	kClient               client.Client
	timeout               time.Duration
	ctx                   context.Context
	kubeconfigPath        string
	conditionCheckTimeout time.Duration
	config                *rest.Config
}

var singletonClient *Client

func GetClient(options *ClientOptions) (*Client, error) {
	if singletonClient != nil && options == nil {
		return singletonClient, nil
	}

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		return nil, fmt.Errorf("KUBECONFIG environment variable not set")
	}

	// Get a Kubernetes rest config
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Printf("Error getting config: %s\n", err)
		return nil, err
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(consolev1.AddToScheme(scheme))
	utilruntime.Must(openshiftv1.AddToScheme(scheme))
	utilruntime.Must(olsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	// Create a new client
	k8sClient, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Printf("Error creating client: %s\n", err)
		return nil, err
	}

	singletonClient = &Client{
		kClient:               k8sClient,
		timeout:               DefaultClientTimeout,
		ctx:                   context.Background(),
		kubeconfigPath:        kubeconfigPath,
		conditionCheckTimeout: DefaultPollTimeout,
		config:                cfg,
	}
	if options != nil {
		singletonClient.conditionCheckTimeout = options.conditionCheckTimeout
	}

	return singletonClient, nil
}

func (c *Client) Create(o client.Object) (err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()

	return c.kClient.Create(ctx, o)
}

func (c *Client) Get(o client.Object) (err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	nsName := client.ObjectKeyFromObject(o)
	return c.kClient.Get(ctx, nsName, o)
}

func (c *Client) Update(o client.Object) (err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	return c.kClient.Update(ctx, o)
}

func (c *Client) Delete(o client.Object) (err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	return c.kClient.Delete(ctx, o)
}

func (c *Client) List(o client.ObjectList, opts ...client.ListOption) (err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	return c.kClient.List(ctx, o, opts...)
}

func (c *Client) WaitForDeploymentRollout(dep *appsv1.Deployment) error {
	return c.WaitForDeploymentCondition(dep, func(dep *appsv1.Deployment) (bool, error) {
		if dep.Generation > dep.Status.ObservedGeneration {
			return false, fmt.Errorf("current generation %d, observed generation %d",
				dep.Generation, dep.Status.ObservedGeneration)
		}
		if dep.Status.UpdatedReplicas != dep.Status.Replicas {
			return false, fmt.Errorf("the number of replicas (%d) does not match the number of updated replicas (%d)",
				dep.Status.Replicas, dep.Status.UpdatedReplicas)
		}
		if dep.Status.UnavailableReplicas != 0 {
			return false, fmt.Errorf("got %d unavailable replicas",
				dep.Status.UnavailableReplicas)
		}
		return true, nil
	})
}

func (c *Client) WaitForDeploymentCondition(dep *appsv1.Deployment, condition func(*appsv1.Deployment) (bool, error)) error {
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, c.conditionCheckTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(dep)
		if err != nil {
			lastErr = fmt.Errorf("failed to get Deployment: %w", err)
			return false, nil
		}
		var conditionMet bool
		conditionMet, lastErr = condition(dep)
		if !conditionMet {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("WaitForDeploymentCondition - waiting for condition of the deployment %s/%s: %w ; last error: %w", dep.GetNamespace(), dep.GetName(), err, lastErr)
	}

	return nil
}

func (c *Client) WaitForConfigMapContainString(cm *corev1.ConfigMap, key, substr string) error {
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, c.conditionCheckTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(cm)
		if err != nil {
			lastErr = fmt.Errorf("failed to get ConfigMap: %w", err)
			return false, nil
		}
		filedata, ok := cm.Data[key]
		if !ok {
			lastErr = fmt.Errorf("key %q not found in ConfigMap", key)
			return false, nil
		}
		if !strings.Contains(filedata, substr) {
			lastErr = fmt.Errorf("substring \"%q\" not found in key %q of ConfigMap", substr, key)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("WaitForConfigMapContainString - waiting for the ConfigMap %s/%s containing the string \"%s\": %w ; last error: %w", cm.GetNamespace(), cm.GetName(), substr, err, lastErr)
	}

	return nil
}

func (c *Client) WaitForServiceCreated(service *corev1.Service) error {
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, c.conditionCheckTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(service)
		if err != nil {
			lastErr = fmt.Errorf("failed to get Service: %w", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("WaitForServiceCreated - waiting for the Service %s/%s to be created: %w ; last error: %w", service.GetNamespace(), service.GetName(), err, lastErr)
	}

	return nil
}

func (c *Client) WaitForSecretCreated(secret *corev1.Secret) error {
	var lastErr error
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, c.conditionCheckTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(secret)
		if err != nil {
			lastErr = fmt.Errorf("failed to get Secret: %w", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("WaitForSecretCreated - waiting for the Secret %s/%s to be created: %w ; last error: %w", secret.GetNamespace(), secret.GetName(), err, lastErr)
	}

	return nil
}

func (c *Client) WaitForObjectCreated(obj client.Object) error {
	var lastErr error
	gvk := obj.GetObjectKind().GroupVersionKind()
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, DefaultPollTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(obj)
		if err != nil {
			lastErr = fmt.Errorf("failed to get Object: %w", err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("WaitForObjectCreated - waiting for the %s %s/%s to be created: %w ; last error: %w", gvk.Kind, obj.GetNamespace(), obj.GetName(), err, lastErr)
	}

	return nil
}

func (c *Client) ForwardPort(serviceName, namespaceName string, port int) (string, func(), error) {
	ctx, cancel := context.WithCancel(c.ctx)
	// #nosec G204
	cmd := exec.CommandContext(ctx, "oc", "port-forward", fmt.Sprintf("service/%s", serviceName), fmt.Sprintf(":%d", port), "-n", namespaceName, "--kubeconfig", c.kubeconfigPath)

	cleanUp := func() {
		cancel()
		_ = cmd.Wait() // wait to clean up resources but ignore returned error since cancel kills the process
	}

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		cleanUp()
		return "", nil, fmt.Errorf("fail to open stdout: %w", err)
	}

	stdErr, err := cmd.StderrPipe()
	if err != nil {
		cleanUp()
		return "", nil, fmt.Errorf("fail to open stderr: %w", err)
	}
	go func() {
		scanner := bufio.NewScanner(stdErr)
		for scanner.Scan() {
			logf.Log.Info(scanner.Text())
		}
		if err != nil {
			logf.Log.Error(err, "scanner error", "stderr", scanner.Err())
		}
	}()

	err = cmd.Start()
	if err != nil {
		cleanUp()
		return "", nil, fmt.Errorf("fail to run command: %w", err)
	}

	scanner := bufio.NewScanner(stdOut)
	if !scanner.Scan() {
		err := scanner.Err()
		if err == nil {
			err = errors.New("got EOF")
		}
		cleanUp()
		return "", nil, fmt.Errorf("fail to read stdout: %w", err)
	}
	output := scanner.Text()

	re := regexp.MustCompile(`^Forwarding from [^:]+:(\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) != 2 {
		cleanUp()
		return "", nil, fmt.Errorf("fail to parse port's value: %q: %w", output, err)
	}
	_, err = strconv.Atoi(matches[1])
	if err != nil {
		cleanUp()
		return "", nil, fmt.Errorf("fail to convert port's value: %q: %w", output, err)
	}

	return fmt.Sprintf("127.0.0.1:%s", matches[1]), cleanUp, nil
}

func (c *Client) createRoute(name, namespace, host string) (func(), error) {
	route := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: routev1.RouteSpec{
			Host: host,
			To: routev1.RouteTargetReference{
				Kind: "Service",
				Name: AppServerServiceName,
			},
			Port: &routev1.RoutePort{
				TargetPort: intstr.FromString("https"),
			},
			TLS: &routev1.TLSConfig{
				Termination: routev1.TLSTerminationPassthrough,
			},
			WildcardPolicy: routev1.WildcardPolicyNone,
		},
	}

	err := c.Create(route)
	if err != nil {
		return nil, err
	}

	return func() {
		err := c.Delete(route)
		if err != nil {
			logf.Log.Error(err, "Error deleting Route")
		}
	}, nil
}
func (c *Client) CreateServiceAccount(namespace, serviceAccount string) (func(), error) {

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount,
			Namespace: namespace,
		},
	}

	err := c.Create(sa)
	if err != nil {
		return nil, err
	}

	return func() {
		err := c.Delete(sa)
		if err != nil {
			logf.Log.Error(err, "Error deleting ServiceAccount")
		}
	}, nil
}

func (c *Client) GetServiceAccountToken(namespace, name string) (string, error) {
	// from kubernetes 1.24+ the token secret should be explicitly created.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "token-serviceaccount-" + name,
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": name,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
	err := c.Create(secret)
	if err != nil {
		return "", err
	}

	var token string
	err = wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, c.conditionCheckTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(secret)
		if err != nil {
			return false, nil
		}
		tokenBytes, ok := secret.Data[corev1.ServiceAccountTokenKey]
		if !ok {
			return false, nil
		}
		token = string(tokenBytes)
		return true, nil

	})
	return token, err
}

func (c *Client) CreateClusterRoleBinding(namespace, serviceAccount, clusterRole string) (func(), error) {
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", serviceAccount, clusterRole),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterRole,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	err := c.Create(clusterRoleBinding)
	if err != nil {
		return nil, err
	}

	return func() {
		err := c.Delete(clusterRoleBinding)
		if err != nil {
			logf.Log.Error(err, "Error deleting ClusterRoleBinding")
		}

	}, nil
}
