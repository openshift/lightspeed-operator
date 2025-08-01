package e2e

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	olsv1alpha1 "github.com/openshift/lightspeed-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PVCName             = "squid-volume-claim"
	SquidDeploymentName = "squid-deployment"
	SquidServiceName    = "squid-service"
	httpsPort           = 3349
	httpPort            = 3128
	squidConfigName     = "squid-config"
	proxyConfigmapName  = "proxy-ca"
)

var _ = Describe("Proxy test", Ordered, Label("Proxy"), func() {

	var cr *olsv1alpha1.OLSConfig
	var err error
	var client *Client
	var cleanUpFuncs []func()
	var forwardCleanUp func()

	const serviceAnnotationKeyTLSSecret = "service.beta.openshift.io/serving-cert-secret-name"
	var httpsClient *HTTPSClient
	var authHeader map[string]string
	var storageClassName string
	var squidHostname string
	var saToken, forwardHost string
	var secret *corev1.Secret

	// Helper function to setup proxy configuration
	setupProxyConfig := func(proxyURL string, proxyCACertRef *corev1.LocalObjectReference) {
		By("modifying the olsconfig to use proxy")
		err = client.Get(cr)
		Expect(err).NotTo(HaveOccurred())

		proxyConfig := &olsv1alpha1.ProxyConfig{
			ProxyURL: proxyURL,
		}
		if proxyCACertRef != nil {
			proxyConfig.ProxyCACertificateRef = proxyCACertRef
		}
		cr.Spec.OLSConfig.ProxyConfig = proxyConfig

		err = client.Update(cr)
		Expect(err).NotTo(HaveOccurred())
	}

	// Helper function to wait for deployment rollout
	waitForAppServerRollout := func() {
		By("wait for application server deployment rollout")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())
	}

	// Helper function to setup port forwarding and HTTPS client
	setupHTTPSClient := func() {
		By("forwarding the HTTPS port to a local port")
		forwardHost, forwardCleanUp, err = client.ForwardPort(AppServerServiceName, OLSNameSpace, AppServerServiceHTTPSPort)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, forwardCleanUp)

		const inClusterHost = "lightspeed-app-server.openshift-lightspeed.svc.cluster.local"
		certificate, ok := secret.Data["tls.crt"]
		Expect(ok).To(BeTrue())
		httpsClient = NewHTTPSClient(forwardHost, inClusterHost, certificate, nil, nil)
		authHeader = map[string]string{"Authorization": "Bearer " + saToken}
	}

	// Helper function to make query request and validate response
	makeQueryRequest := func() {
		By("creating a query request")
		reqBody := []byte(`{"query": "what is Openshift?"}`)
		var resp *http.Response
		Expect(httpsClient).NotTo(BeNil(), "httpsClient is nil, cannot make POST request")
		resp, err = httpsClient.PostJson("/v1/query", reqBody, authHeader)
		if err != nil {
			fmt.Printf("Error during POST request: %v\n", err)
		}
		Expect(err).NotTo(HaveOccurred())

		if resp != nil {
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				fmt.Printf("Error reading response body: %v\n", err)
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(body).NotTo(BeEmpty())
		} else {
			fmt.Println("Response is nil, possible panic point")
		}
	}

	BeforeAll(func() {
		client, err = GetClient(nil)
		Expect(err).NotTo(HaveOccurred())

		storageClassName = StorageClassNameCI
		defaultStorageClass, err := client.GetDefaultStorageClass()
		if err == nil {
			storageClassName = defaultStorageClass.Name
		}
		if defaultStorageClass == nil {
			storageClassName = StorageClassNameLocal
			By("Cannot find the CI storage class, using local storage class for testing, this test will be flaky if cluster has more than 1 worker node")
			By("Creating a StorageClass")
			cleanUpStorageClass, err := client.CreateStorageClass(storageClassName)
			Expect(err).NotTo(HaveOccurred())
			cleanUpFuncs = append(cleanUpFuncs, cleanUpStorageClass)

		}

		By("Creating a PersistentVolumeClaim")
		cleanUpPV, err := client.CreatePersistentVolumeClaim(PVCName, storageClassName, resource.MustParse("1Gi"))
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUpPV)

		By("create configmap for squid using the squid.conf file")

		squidConfPath := filepath.Join("..", "utils", "squid.conf")
		squidConfData, err := os.ReadFile(squidConfPath)
		Expect(err).NotTo(HaveOccurred())

		squidConfig := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      squidConfigName,
				Namespace: OLSNameSpace,
			},
			Data: map[string]string{
				"squid": string(squidConfData),
			},
		}
		err = client.Create(squidConfig)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForObjectCreated(squidConfig)
		Expect(err).NotTo(HaveOccurred())

		By("create service for squid")
		squidService := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        SquidServiceName,
				Namespace:   OLSNameSpace,
				Labels:      map[string]string{"app": "squid"},
				Annotations: map[string]string{"service.beta.openshift.io/serving-cert-secret-name": "squid-service-tls"},
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Port: 3349,
						Name: "squid-https",
					},
					{
						Port: 3128,
						Name: "squid",
					},
				},
				Selector: map[string]string{
					"app": "squid",
				},
			},
		}
		err = client.Create(squidService)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForServiceCreated(squidService)
		Expect(err).NotTo(HaveOccurred())

		By("create deployment for squid")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SquidDeploymentName,
				Namespace: OLSNameSpace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: func() *int32 { i := int32(1); return &i }(),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "squid"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": "squid"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "squid",
								Image: "ubuntu/squid:edge",
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: httpsPort,
										Name:          "squid-https",
										Protocol:      corev1.ProtocolTCP,
									},
									{
										ContainerPort: httpPort,
										Name:          "squid",
										Protocol:      corev1.ProtocolTCP,
									},
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "squid-config-volume",
										MountPath: "/etc/squid/squid.conf",
										SubPath:   "squid.conf",
									},
									{
										Name:      "squid-data",
										MountPath: "/var/spool/squid",
									},
									{
										Name:      "squid-ssl",
										MountPath: "/etc/squid/ssl_ca",
										ReadOnly:  true,
									},
									{
										Name:      "squid-logs",
										MountPath: "/var/log/squid",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							{
								Name: "squid-config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: squidConfigName,
										},
										Items: []corev1.KeyToPath{
											{
												Key:  "squid",
												Path: "squid.conf",
											},
										},
									},
								},
							},
							{
								Name: "squid-data",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: PVCName,
									},
								},
							},
							{
								Name: "squid-ssl",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: "squid-service-tls",
										Items: []corev1.KeyToPath{
											{
												Key:  "tls.crt",
												Path: "tls.crt",
											},
											{
												Key:  "tls.key",
												Path: "tls.key",
											},
										},
									},
								},
							},
							{
								Name: "squid-logs",
								VolumeSource: corev1.VolumeSource{
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
			},
		}
		err = client.Create(deployment)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

		By("copy contents of openshift-service-ca.crt to proxy-ca")
		serviceCAConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-service-ca.crt",
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(serviceCAConfigMap)
		Expect(err).NotTo(HaveOccurred())
		serviceCACrt, ok := serviceCAConfigMap.Data["service-ca.crt"]
		Expect(ok).To(BeTrue())

		configmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      proxyConfigmapName,
				Namespace: OLSNameSpace,
			},
			Data: map[string]string{
				"proxy-ca.crt": serviceCACrt,
			},
		}
		err = client.Create(configmap)
		Expect(err).NotTo(HaveOccurred())
		err = client.WaitForObjectCreated(configmap)
		Expect(err).NotTo(HaveOccurred())

		By("get the squid service hostname")
		squidService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SquidServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Get(squidService)
		Expect(err).NotTo(HaveOccurred())

		if len(squidService.Status.LoadBalancer.Ingress) > 0 {
			squidHostname = squidService.Status.LoadBalancer.Ingress[0].Hostname
		} else {
			squidHostname = fmt.Sprintf("%s.%s.svc.cluster.local", SquidServiceName, OLSNameSpace)
		}
		Expect(squidHostname).NotTo(BeEmpty())

		By("Creating a OLSConfig CR")
		cr, err = generateOLSConfig()
		Expect(err).NotTo(HaveOccurred())
		err = client.Create(cr)
		Expect(err).NotTo(HaveOccurred())

		By("create a service account for OLS user")
		testSAName := "test-sa"
		cleanUp, err := client.CreateServiceAccount(OLSNameSpace, testSAName)
		Expect(err).NotTo(HaveOccurred())
		cleanUpFuncs = append(cleanUpFuncs, cleanUp)

		By("create a role binding for OLS user accessing query API")
		queryAccessClusterRole := "lightspeed-operator-query-access"
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
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForSecretCreated(secret)
		Expect(err).NotTo(HaveOccurred())

		By("wait for application server deployment rollout")
		deployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AppServerDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.WaitForDeploymentRollout(deployment)
		Expect(err).NotTo(HaveOccurred())

	})

	AfterAll(func() {

		By("Deleting the OLSConfig CR")
		if cr != nil {
			client.Delete(cr)
		}

		By("Deleting the proxy configmap")
		configmap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      proxyConfigmapName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Delete(configmap)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the squid-config configmap")
		configmap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      squidConfigName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Delete(configmap)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the PVC")
		PVC := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PVCName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Delete(PVC)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the proxy service")
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SquidServiceName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Delete(service)
		Expect(err).NotTo(HaveOccurred())

		By("Deleting the squid deployment")
		deployment := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SquidDeploymentName,
				Namespace: OLSNameSpace,
			},
		}
		err = client.Delete(deployment)
		Expect(err).NotTo(HaveOccurred())

		for _, cleanUpFunc := range cleanUpFuncs {
			cleanUpFunc()
		}
	})

	It("should be able to query the application server with http proxy", func() {
		setupProxyConfig("http://"+squidHostname+":"+strconv.Itoa(httpPort), nil)
		waitForAppServerRollout()
		setupHTTPSClient()
		makeQueryRequest()
	})

	It("should be able to query the application server with https proxy", func() {
		setupProxyConfig("https://"+squidHostname+":"+strconv.Itoa(httpsPort), &corev1.LocalObjectReference{
			Name: "proxy-ca",
		})
		waitForAppServerRollout()
		setupHTTPSClient()
		makeQueryRequest()
	})
})
