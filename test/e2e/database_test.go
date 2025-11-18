package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StorageClassNameLocal = "local-storage"
	StorageClassNameCI    = "gp3-csi"
	PvName                = "lightspeed-postgres-pv"
	PostgresPVCName       = "lightspeed-postgres-pvc"
)

var _ = Describe("Database Persistency", Ordered, Label("Database-Persistency"), func() {

	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()

	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	const testSAName = "test-sa"
	const queryAccessClusterRole = "lightspeed-operator-query-access"
	const inClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"
	var saToken, forwardHost string
	var httpsClient *HTTPSClient
	var authHeader map[string]string
	var storageClassName string
	var certificate []byte

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		By("get the default storage class")
		storageClassName = StorageClassNameCI
		defaultStorageClass, err := client.GetDefaultStorageClass()
		if err == nil {
			storageClassName = defaultStorageClass.Name
		}

		By("create a storage class with its PV for testing")
		if defaultStorageClass == nil {
			storageClassName = StorageClassNameLocal
			By("Cannot find the default storage class, using local storage class for testing, this test will be flaky if cluster has more than 1 worker node")
			By("Creating a StorageClass")
			cleanUpStorageClass, err := client.CreateStorageClass(storageClassName)
			Expect(err).NotTo(HaveOccurred())
			cleanUpFuncs = append(cleanUpFuncs, cleanUpStorageClass)

			By("Creating a PersistentVolume")
			cleanUpPV, err := client.CreatePersistentVolume(PvName, storageClassName, resource.MustParse("1Gi"))
			Expect(err).NotTo(HaveOccurred())
			cleanUpFuncs = append(cleanUpFuncs, cleanUpPV)
		}

		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		cr.Spec.OLSConfig.Storage = &olsv1alpha1.Storage{
			Size:  resource.MustParse("768Mi"),
			Class: storageClassName,
		}
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())

		By("create a service account for OLS user")
		cleanUp, err := client.CreateServiceAccount(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("create a role binding for OLS user accessing query API")
		cleanUp, err = client.CreateClusterRoleBinding(OLSNameSpace, testSAName, queryAccessClusterRole)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("fetch the service account tokens")
		saToken, err = client.GetServiceAccountToken(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for the application service created")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForServiceCreated(service)
		Expect(err).NotTo(HaveOccurred())

		By("check the secret holding TLS certificates is created")
		secretName, ok := service.ObjectMeta.Annotations[serviceAnnotationKeyTLSSecret]
		Expect(ok).To(BeTrue())
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForSecretCreated(secret)
		Expect(err).NotTo(HaveOccurred())

		By("wait for application server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("forwarding the HTTPS port to a local port")
		forwardHost, cleanUp, err = client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		certificate, ok = secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
		httpsClient = NewHTTPSClient(forwardHost, inClusterHost, certificate, nil, nil)
		authHeader = map[string]string{"Authorization": "Bearer " + saToken}

	})

	AfterAll(func() {
		err = mustGather("database_test")
		Expect(err).NotTo(HaveOccurred())
		By("Deleting the OLSConfig CR")
		if cr != nil {
			client.Delete(cr)
		}

		for _, cleanUpFunc := range cleanUpFuncs {
			cleanUpFunc()
		}
	})

	It("should persist data in the database", FlakeAttempts(5), func() {
		By("checking the PVC is created")
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresPVCName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(pvc)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the database to be ready")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgresDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("Get the PostgreSQL pod")
		databasePod, err := getDatabasePod(client, deployment)
		Expect(err).NotTo(HaveOccurred())

		By("find out the database pod's node name")
		fmt.Fprintf(GinkgoWriter, "Database pod's node name: %s\n", databasePod.Spec.NodeName)

		By("send query to OLS, generate a conversation record in database")
		reqBody := []byte(`{"query": "what is latest version of Openshift?"}`)
		var resp *http.Response

		// Use Eventually to handle potential EOF errors and restart port forwarding if needed
		Eventually(func() error {
			var err, pfErr error
			resp, err = httpsClient.PostJson("/v1/query", reqBody, authHeader)
			if err != nil && strings.Contains(err.Error(), "EOF") {
				By("EOF error detected, restarting port forwarding")
				var portForwardCleanup func()
				// Restart port forwarding
				forwardHost, portForwardCleanup, pfErr = client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
				if pfErr != nil {
					// log the portforward error and return error from http post request
					fmt.Fprintf(GinkgoWriter, "failed to restart port forwarding: %s \n", pfErr)
					return err
				}
				cleanUpFuncs = append(cleanUpFuncs, portForwardCleanup)
				httpsClient = NewHTTPSClient(forwardHost, inClusterHost, certificate, nil, nil)
			}
			return err
		}, 1*time.Minute, 5*time.Second).Should(Succeed())

		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).NotTo(BeEmpty())

		By("check the conversation record is created in database")
		sqlCommand := "SELECT COUNT(*) FROM cache;"
		cmd := exec.CommandContext(context.TODO(), "oc", "--kubeconfig", client.kubeconfigPath, "exec", databasePod.Name, "-n", OLSNameSpace, "--", "psql", "-c", sqlCommand)
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to execute SQL command: %s", string(output))
		// read the second line of the output, line splitted by \n
		// output has these 6 lines:
		// " count ", "-------", "     3", "(1 row)", "", ""
		lines := strings.Split(string(output), "\n")
		Expect(lines).To(HaveLen(6), "Expected 6 lines of output")
		count, err := strconv.Atoi(strings.TrimSpace(lines[2]))
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(BeNumerically(">", 0), "Expected to have at least 1 conversation record")

		By("restart the database pod")
		err = client.Delete(&databasePod)
		Expect(err).NotTo(HaveOccurred())
		// wait for the pod to be deleted
		Eventually(func() bool {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databasePod.Name,
					Namespace: OLSNameSpace,
				},
			}
			err := client.Get(pod)
			return k8serrors.IsNotFound(err)
		}, 2*time.Minute, 2*time.Second).Should(BeTrue())

		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("Get the PostgreSQL pod")
		databasePod, err = getDatabasePod(client, deployment)
		Expect(err).NotTo(HaveOccurred())

		By("find out the database pod's node name")
		fmt.Fprintf(GinkgoWriter, "Database pod's node name: %s\n", databasePod.Spec.NodeName)

		By("check the conversation records are still in database")
		cmd = exec.CommandContext(context.TODO(), "oc", "--kubeconfig", client.kubeconfigPath, "exec", databasePod.Name, "-n", OLSNameSpace, "--", "psql", "-c", sqlCommand)
		output, err = cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), "Failed to execute SQL command: %s", string(output))
		lines = strings.Split(string(output), "\n")
		Expect(lines).To(HaveLen(6), "Expected 6 lines of output")
		newCount, err := strconv.Atoi(strings.TrimSpace(lines[2]))
		Expect(err).NotTo(HaveOccurred())
		Expect(newCount).To(BeNumerically(">", 0), "Expected to have at least 1 conversation record")
		Expect(newCount).To(Equal(count), "Expected the count to be the same")
	})

})

func getDatabasePod(client *Client, deployment *appsv1.Deployment) (corev1.Pod, error) {
	selector := labels.Set(deployment.Spec.Selector.MatchLabels).AsSelector()
	podList := &corev1.PodList{}
	err := client.List(podList, &k8sclient.ListOptions{
		Namespace:     OLSNameSpace,
		LabelSelector: selector,
	})
	if err != nil {
		return corev1.Pod{}, err
	}
	if len(podList.Items) == 0 {
		return corev1.Pod{}, fmt.Errorf("no database pod found")
	}
	// get the latest pod by creation timestamp
	latestPod := podList.Items[0]
	for _, pod := range podList.Items[1:] {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		if pod.CreationTimestamp.After(latestPod.CreationTimestamp.Time) {
			latestPod = pod
		}
	}
	return latestPod, nil
}
