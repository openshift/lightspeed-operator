package e2e

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	imagev1 "github.com/openshift/api/image/v1"
	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	openshiftv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
)

const (
	// DefaultTimeout is the default timeout for client operations
	DefaultClientTimeout = 30 * time.Second
	// DefaultPollInterval is the default interval for polling
	DefaultPollInterval = 5 * time.Second
	// DefaultPollTimeout is the default timeout for polling
	DefaultPollTimeout = 15 * time.Minute
)

type ClientOptions struct {
	conditionCheckTimeout time.Duration
}

type Client struct {
	kClient               client.Client
	clientset             kubernetes.Interface
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
	utilruntime.Must(imagev1.AddToScheme(scheme))
	utilruntime.Must(imageregistryv1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	// Create a new client
	k8sClient, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		fmt.Printf("Error creating client: %s\n", err)
		return nil, err
	}

	// Create Kubernetes clientset for port forwarding
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Printf("Error creating clientset: %s\n", err)
		return nil, err
	}

	singletonClient = &Client{
		kClient:               k8sClient,
		clientset:             clientset,
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

// Update fetches the latest version of an object, applies modifications via the modifier function,
// and automatically retries on conflicts. This handles the common race condition where the operator
// reconciles while the test is updating the same resource.
func (c *Client) Update(obj client.Object, modifier func(client.Object) error) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get the latest version
		ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
		defer cancel()

		key := client.ObjectKeyFromObject(obj)
		if err := c.kClient.Get(ctx, key, obj); err != nil {
			return err
		}

		// Apply modifications
		if err := modifier(obj); err != nil {
			return err
		}

		// Update with fresh version
		return c.kClient.Update(ctx, obj)
	})
}

func (c *Client) Delete(o client.Object) (err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	return c.kClient.Delete(ctx, o)
}

// DeleteAndWait deletes an object and waits for it to be fully removed from the cluster.
// This ensures complete cleanup before tests proceed, preventing resource pollution from
// resources with owner references that trigger cascade deletion (e.g., OLSConfig CR).
// Returns nil if the object is already deleted (IsNotFound).
func (c *Client) DeleteAndWait(obj client.Object, timeout time.Duration) error {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	namespace := obj.GetNamespace()
	name := obj.GetName()

	// Delete the object
	if err := c.Delete(obj); err != nil {
		if k8serrors.IsNotFound(err) {
			// Already deleted, nothing to wait for
			return nil
		}
		return fmt.Errorf("failed to delete %s %s/%s: %w", kind, namespace, name, err)
	}

	// Wait for complete deletion
	err := wait.PollUntilContextTimeout(c.ctx, DefaultPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(obj)
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			// Unexpected error - keep polling
			return false, nil
		}
		// Object still exists, keep waiting
		return false, nil
	})

	if err != nil {
		// Add more context when timeout occurs
		return fmt.Errorf("timeout waiting for %s %s/%s to be deleted after %v: %w",
			kind, namespace, name, timeout, err)
	}

	return nil
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
			_ = c.ShowUnavailablePodsOfDeployment(dep)
			return false, fmt.Errorf("got %d unavailable replicas",
				dep.Status.UnavailableReplicas)
		}
		return true, nil
	})
}

func (c *Client) WaitForDeploymentNextGeneration(dep *appsv1.Deployment, oldGen int64) error {
	return c.WaitForDeploymentCondition(dep, func(dep *appsv1.Deployment) (bool, error) {
		if dep.Status.ObservedGeneration <= oldGen {
			return false, fmt.Errorf("observed generation %d, old generation %d",
				dep.Status.ObservedGeneration, oldGen)
		}
		return true, nil
	})
}

func (c *Client) isPodTerminating(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
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

func (c *Client) ShowUnavailablePodsOfDeployment(dep *appsv1.Deployment) error {
	err := c.Get(dep)
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", dep.GetNamespace(), dep.GetName(), err)
	}

	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return fmt.Errorf("failed to parse deployment selector: %w", err)
	}

	pods := &corev1.PodList{}
	err = c.List(pods, client.InNamespace(dep.GetNamespace()), client.MatchingLabelsSelector{
		Selector: selector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods for deployment %s/%s: %w", dep.GetNamespace(), dep.GetName(), err)
	}

	if len(pods.Items) == 0 {
		logf.Log.Info("No pods found for deployment", "deployment", fmt.Sprintf("%s/%s", dep.GetNamespace(), dep.GetName()))
		return nil
	}

	// Find unavailable pods
	unavailablePods := []corev1.Pod{}
	for _, pod := range pods.Items {
		isAvailable := false

		// A pod is considered available if it's running and ready
		if pod.Status.Phase == corev1.PodRunning && !c.isPodTerminating(&pod) {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					isAvailable = true
					break
				}
			}
		}

		if !isAvailable {
			unavailablePods = append(unavailablePods, pod)
		}
	}

	// Log unavailable pods and their status messages
	if len(unavailablePods) == 0 {
		logf.Log.Info("All pods are available for deployment", "deployment", fmt.Sprintf("%s/%s", dep.GetNamespace(), dep.GetName()))
	} else {
		logf.Log.Info("Found unavailable pods", "deployment", fmt.Sprintf("%s/%s", dep.GetNamespace(), dep.GetName()), "count", len(unavailablePods))

		for _, pod := range unavailablePods {
			logf.Log.Info("Unavailable pod details",
				"pod", pod.Name,
				"phase", pod.Status.Phase,
				"reason", pod.Status.Reason,
				"message", pod.Status.Message,
			)

			// Show container statuses
			for _, containerStatus := range pod.Status.ContainerStatuses {
				logf.Log.Info("Container status",
					"pod", pod.Name,
					"container", containerStatus.Name,
					"ready", containerStatus.Ready,
					"restartCount", containerStatus.RestartCount,
				)

				if containerStatus.State.Waiting != nil {
					logf.Log.Info("Container waiting",
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", containerStatus.State.Waiting.Reason,
						"message", containerStatus.State.Waiting.Message,
					)
				}

				if containerStatus.State.Terminated != nil {
					logf.Log.Info("Container terminated",
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", containerStatus.State.Terminated.Reason,
						"message", containerStatus.State.Terminated.Message,
						"exitCode", containerStatus.State.Terminated.ExitCode,
					)
				}

				if containerStatus.LastTerminationState.Terminated != nil {
					logf.Log.Info("Container last termination",
						"pod", pod.Name,
						"container", containerStatus.Name,
						"reason", containerStatus.LastTerminationState.Terminated.Reason,
						"message", containerStatus.LastTerminationState.Terminated.Message,
						"exitCode", containerStatus.LastTerminationState.Terminated.ExitCode,
					)
				}
			}

			// Show pod conditions
			for _, condition := range pod.Status.Conditions {
				if condition.Status != corev1.ConditionTrue {
					logf.Log.Info("Pod condition not met",
						"pod", pod.Name,
						"type", condition.Type,
						"status", condition.Status,
						"reason", condition.Reason,
						"message", condition.Message,
					)
				}
			}
		}
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
	return c.ForwardPortV2(serviceName, namespaceName, port)
}

// ForwardPortV2 is the new implementation using service and pod selector, select running but not terminating pods
func (c *Client) ForwardPortV2(serviceName, namespaceName string, port int) (string, func(), error) {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespaceName,
		},
	}
	err := c.Get(service)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get service %s/%s: %w", namespaceName, serviceName, err)
	}

	var targetPort int32 = 0
	if len(service.Spec.Ports) == 0 {
		return "", nil, fmt.Errorf("service %s/%s has no ports defined", namespaceName, serviceName)
	}

	// Find the port that matches the requested port
	for _, svcPort := range service.Spec.Ports {
		if svcPort.Port == int32(port) {
			if svcPort.TargetPort.IntVal != 0 {
				targetPort = svcPort.TargetPort.IntVal
			} else {
				targetPort = svcPort.Port
			}
			break
		}
	}

	if targetPort == 0 {
		return "", nil, fmt.Errorf("port %d not found on service %s/%s", port, namespaceName, serviceName)
	}

	selector := labels.SelectorFromSet(service.Spec.Selector)

	pods := &corev1.PodList{}
	err = c.List(pods, client.InNamespace(namespaceName), client.MatchingLabelsSelector{
		Selector: selector,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list pods with selector %s: %w", selector, err)
	}

	if len(pods.Items) == 0 {
		return "", nil, fmt.Errorf("no pods found for service %s/%s with selector %s", namespaceName, serviceName, selector)
	}

	// Find the best pod to forward to (running and ready)
	var targetPod *corev1.Pod
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			if c.isPodTerminating(&pod) {
				continue
			}
			isReady := true
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
					isReady = false
					break
				}
			}
			if isReady {
				targetPod = &pod
				break
			}
		}
	}

	if targetPod == nil {
		return "", nil, fmt.Errorf("no ready pods found for service %s/%s", namespaceName, serviceName)
	}

	logf.Log.Info("Port forwarding via service", "service", fmt.Sprintf("%s/%s", namespaceName, serviceName), "pod", targetPod.Name, "port", targetPort)
	return c.ForwardPortToPod(targetPod.Name, namespaceName, int(targetPort))
}

func (c *Client) ForwardPortToPod(podName, namespaceName string, port int) (string, func(), error) {

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespaceName).
		Name(podName).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(c.config)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create SPDY round tripper: %w", err)
	}

	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(c.ctx)
	ports := []string{fmt.Sprintf("0:%d", port)}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	pf, err := portforward.New(
		dialer,
		ports,
		ctx.Done(),
		readyCh,
		nil, // stdout ignored
		nil, // stderr ignored
	)
	if err != nil {
		cancel()
		return "", nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	// port forwarding routine
	go func() {
		if err := pf.ForwardPorts(); err != nil {
			select {
			case errCh <- err:
				logf.Log.Error(err, "Port forwarding error")
			default:
			}
		}
	}()

	// Wait for the port forward to be ready or fail
	select {
	case <-readyCh:
		// Port forwarding is ready
		ports, err := pf.GetPorts()
		if err != nil {
			cancel()
			return "", nil, fmt.Errorf("failed to get forwarded ports: %w", err)
		}

		if len(ports) == 0 {
			cancel()
			return "", nil, fmt.Errorf("no ports were forwarded")
		}

		localPort := ports[0].Local
		address := fmt.Sprintf("127.0.0.1:%d", localPort)

		cleanUp := func() {
			cancel()
		}

		logf.Log.Info("Port forwarding established", "pod", fmt.Sprintf("%s/%s", namespaceName, podName), "local", address, "remote", port)
		return address, cleanUp, nil

	case err := <-errCh:
		cancel()
		return "", nil, fmt.Errorf("port forwarding failed: %w", err)

	case <-time.After(30 * time.Second):
		cancel()
		return "", nil, fmt.Errorf("timeout waiting for port forward to be ready")
	}
}

// ForwardPortV1 is the original implementation using oc command line tool, selecting running pods
func (c *Client) ForwardPortV1(serviceName, namespaceName string, port int) (string, func(), error) {
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

// CheckErrorAndRestartPortForwardService checks if the error is EOF and restarts the port forwarding service
// Port forwarding should work on next attempt of the test case
func (c *Client) CheckErrorAndRestartPortForwardService(err error, serviceName, namespace string, port int, forwardHost *string, cleanUpFuncs *[]func()) {
	var fpErr error
	var cleanUp func()
	var forwardHostNew string
	if err == nil {
		return
	}
	if !strings.Contains(err.Error(), "EOF") {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "CheckErrorAndRestartPortForwardService skips non-EOF error: %s \n", err)
		return
	}
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "EOF error detected, restarting port forwarding\n")
	forwardHostNew, cleanUp, fpErr = c.ForwardPort(serviceName, namespace, port)
	if fpErr != nil {
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "failed to restart port forwarding: %s \n", fpErr)
		return
	}
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "restarted port forwarding\n")
	*forwardHost = forwardHostNew
	*cleanUpFuncs = append(*cleanUpFuncs, cleanUp)
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
		if k8serrors.IsAlreadyExists(err) {
			logf.Log.Error(err, "Error ServiceAccount for test already exists")
		} else {
			return nil, err
		}
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
		// ROSA clusters have more restricted permissions.
		// Deleting ClusterRoleBindings is not allowed.
		if k8serrors.IsAlreadyExists(err) {
			logf.Log.Error(err, "ClusterRoleBindings for test already exists (happens for ROSA clusters)")
		} else {
			return nil, err
		}
	}

	return func() {
		err := c.Delete(clusterRoleBinding)
		if err != nil {
			logf.Log.Error(err, "Error deleting ClusterRoleBinding")
		}

	}, nil
}

func (c *Client) UpgradeOperator(namespace string) error {
	ctx, cancel := context.WithCancel(c.ctx)
	bundleImage := os.Getenv("BUNDLE_IMAGE")
	cmd := exec.CommandContext(ctx, "operator-sdk", "run", "bundle-upgrade", bundleImage, "--namespace", namespace, "--timeout", "20m", "--verbose", "--kubeconfig", c.kubeconfigPath)

	cleanUp := func() {
		cancel()
		_ = cmd.Wait() // wait to clean up resources but ignore returned error since cancel kills the process
	}

	err := cmd.Run()
	if err != nil {
		cleanUp()
		return fmt.Errorf("fail to run upgrade command. Please check upgrade version is different from initial: %w", err)
	}
	return nil
}

func (c *Client) GetStorageClass(name string) (*storagev1.StorageClass, error) {
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	err := c.Get(storageClass)
	if err != nil {
		return nil, err
	}
	return storageClass, nil
}

func (c *Client) GetDefaultStorageClass() (*storagev1.StorageClass, error) {
	storageClassList := &storagev1.StorageClassList{}
	err := c.List(storageClassList)
	if err != nil {
		return nil, err
	}
	for _, sc := range storageClassList.Items {
		logf.Log.Info("StorageClass", "name", sc.Name, "annotations", sc.Annotations)
		if sc.Annotations != nil {
			if defaultAnnotation, exists := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; exists && defaultAnnotation == "true" {
				return &sc, nil
			}
		}
	}
	return nil, fmt.Errorf("no default storage class found")
}

func (c *Client) CreateStorageClass(name string) (func(), error) {
	volumeBindingMode := storagev1.VolumeBindingWaitForFirstConsumer
	storageClass := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			},
		},
		Provisioner:       "kubernetes.io/no-provisioner",
		VolumeBindingMode: &volumeBindingMode,
	}

	err := c.Create(storageClass)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logf.Log.Error(err, "StorageClass for test already exists")
		} else {
			return nil, err
		}
	}

	return func() {
		err := c.Delete(storageClass)
		if err != nil {
			logf.Log.Error(err, "Error deleting StorageClass")
		}
	}, nil
}

func (c *Client) CreatePersistentVolume(name, storageClassName string, volumeSize resource.Quantity) (func(), error) {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: storageClassName,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: volumeSize,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				Local: &corev1.LocalVolumeSource{
					Path: "/tmp",
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "kubernetes.io/os",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"linux"},
								},
							},
						},
					},
				},
			},
		},
	}

	err := c.Create(pv)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logf.Log.Error(err, "PersistentVolume for test already exists")
		} else {
			return nil, err
		}
	}

	return func() {
		err := c.Delete(pv)
		if err != nil {
			logf.Log.Error(err, "Error deleting PersistentVolume")
		}
	}, nil
}

func (c *Client) CreatePVC(name, storageClassName string, volumeSize resource.Quantity) (func(), error) {
	pv := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: OLSNameSpace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: volumeSize,
				},
			},
		},
	}

	err := c.Create(pv)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logf.Log.Error(err, "PersistentVolumeClaim for test already exists")
		} else {
			return nil, err
		}
	}

	return func() {
		err := c.Delete(pv)
		if err != nil {
			logf.Log.Error(err, "Error deleting PersistentVolumeClaim")
		}
	}, nil
}

func (c *Client) CreateDockerRegistrySecret(namespace, name, server, username, password, email string) (func(), error) {
	auth := base64.StdEncoding.EncodeToString(
		[]byte(username + ":" + password),
	)

	dockerConfig := map[string]any{
		"auths": map[string]any{
			server: map[string]string{
				"username": username,
				"password": password,
				"email":    email,
				"auth":     auth,
			},
		},
	}

	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}
	if err := c.Create(secret); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logf.Log.Error(err, "Secret %s/%s already exists", namespace, name)
		} else {
			return nil, err
		}
	}

	if err := c.WaitForSecretCreated(secret); err != nil {
		return nil, err
	}

	return func() {
		err := c.Delete(secret)
		if err != nil {
			logf.Log.Error(err, "Error deleting secret %s/%s", namespace, name)
		}
	}, nil
}
