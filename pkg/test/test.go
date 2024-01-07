package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func testURL(errName string, url string, resp *http.Response, err error, expectedRespBody string) error {
	if err != nil {
		return fmt.Errorf("Failed to connect to %s %s: %s", errName, url, err)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to ready body from %s %s: %s", errName, url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s returned non-200 error code %d: %s", errName, url, resp.StatusCode, string(respBody))
	}
	if expectedRespBody == "" {
		return nil
	}
	if expectedRespBody != string(respBody) {
		return fmt.Errorf("%s %s returned unexpected body %s instead of expected body %s", errName, url, string(respBody), expectedRespBody)
	}
	return nil
}

// MergedValues is the subset of the helm values.yaml that need to be inspected to execute the test
type MergedValues struct {
	FullnameOverride string            `json:"fullnameOverride"`
	TestFile         TestFile          `json:"testFile"`
	Deployment       DeploymentValues  `json:"deployment"`
	StatefulSet      StatefulSetValues `json:"statefulset"`
}

// TestFile is the location and contents of a test file to submit to the services as part of the test
type TestFile struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

// DeploymentValues is the subset of the helm values.yaml deployment: field that need to be inspected to execute the test
type DeploymentValues struct {
	Ingress DeploymentIngressValues `json:"ingress"`
}

// DeploymentValues is the subset of the helm values.yaml deployment.ingress: field that need to be inspected to execute the test
type DeploymentIngressValues struct {
	Hostname string                       `json:"hostname"`
	TLS      []DeploymentIngressTLSValues `json:"tls"`
}

// DeploymentValues is the subset of the helm values.yaml deployment.ingress.tls: field that need to be inspected to execute the test
type DeploymentIngressTLSValues struct {
}

// DeploymentValues is the subset of the helm values.yaml statefulset: field that need to be inspected to execute the test
type StatefulSetValues struct {
	NodePortHostname string `json:"nodePortHostname"`
}

func portForward(ctx context.Context, k8sConfig *rest.Config, namespace, pod string, ports []string, f func() error) error {
	portForwardURL, err := url.Parse(k8sConfig.Host)
	if err != nil {
		return errors.Wrap(err, "Failed to parse Kubernetes server URL")
	}
	portForwardURL.Path = filepath.Join(portForwardURL.Path, k8sConfig.APIPath, "/api/v1/namespaces/", namespace, "/pods/", pod, "/portforward")

	roundTripper, upgrader, err := spdy.RoundTripperFor(k8sConfig)
	if err != nil {
		return errors.Wrapf(err, "Failed to create SPDY round-tripper for port-forward at URL %s", portForwardURL)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, portForwardURL)

	log.Println("Beginning port-forward...")

	ready := make(chan struct{})
	stop := make(chan struct{})
	defer close(stop)
	errChan := make(chan error)
	forwarder, err := portforward.New(dialer, ports, stop, ready, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	defer forwarder.Close()
	go func() {
		errChan <- forwarder.ForwardPorts()
	}()
	select {
	case err = <-errChan:
		return err
	case <-ctx.Done():
		return context.Canceled
	case <-ready:
	}
	return f()
}

// Config is the configuration for a test
type Config struct {
	// HTTP is the HTTP client to use, which can be configured to use a proxy, include mTLS certificates, etc
	HTTP *http.Client
	// K8sConfig is the Kubernetes configuration to contact the cluster that the helm chart was deployed to
	K8sConfig *rest.Config
	// ReleaseNamespace is the Kubernetes namespace that the helm chart was deployed to
	ReleaseNamespace string
	// ReleaseName is the name of the helm chart release
	ReleaseName string
	// MergedValues is the parsed complete values.yaml from the helm release
	MergedValues *MergedValues
	// PortForwardLocalPort is the local port to use to test port-forwarding
	PortForwardLocalPort int
	// IngressHostname is the hostname to use instead of the ingress hostname from the values.yaml to contact the services over ingress.
	// If non-empty, requests to the ingress will use this as the hostname in the URL, and the deployment.ingress.hostname as the Host header/TLS server name.
	// This can be used to test ingress when DNS is not configured.
	IngressHostname string
	// IngressTLS indicates to use TLS (HTTPS) for testing the ingress, regardless of what is set in the helm values.yaml
	IngressTLS bool
}

func (cfg *Config) K8sClient() (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(cfg.K8sConfig)
}

func (cfg *Config) Fullname() string {
	fullname := cfg.MergedValues.FullnameOverride
	if fullname != "" {
		return fullname
	}
	if strings.Contains(cfg.ReleaseName, "k8s-smoke-test") {
		return cfg.ReleaseName
	}
	return cfg.ReleaseName + "-k8s-smoke-test"
}

func (cfg *Config) PickDeploymentPod(ctx context.Context, k8sClient *kubernetes.Clientset, fullname string) (*corev1.Pod, error) {
	deploymentService, err := k8sClient.CoreV1().Services(cfg.ReleaseNamespace).Get(ctx, fmt.Sprintf("%s-deployment", fullname), metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get Deployment Service")
	}
	deploymentPods, err := k8sClient.CoreV1().Pods(cfg.ReleaseNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(deploymentService.Spec.Selector),
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list Deployment Pods")
	}
	if len(deploymentPods.Items) == 0 {
		return nil, errors.New("No deployment pods were present")
	}
	return &deploymentPods.Items[0], nil
}

func (cfg *Config) GetStatefulSetService(ctx context.Context, k8sClient *kubernetes.Clientset, fullname string) (*corev1.Service, error) {
	statefulSetService, err := k8sClient.CoreV1().Services(cfg.ReleaseNamespace).Get(ctx, fullname+"-statefulset", metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get StatefulSet Service")
	}
	return statefulSetService, nil
}

func TestPortForward(ctx context.Context, cfg *Config, pod *corev1.Pod) error {
	return portForward(ctx, cfg.K8sConfig, cfg.ReleaseNamespace, pod.Name, []string{fmt.Sprintf("%d:8080", cfg.PortForwardLocalPort)}, func() error {
		portForwardURL := fmt.Sprintf("http://localhost:%d/rwx/%s", cfg.PortForwardLocalPort, cfg.MergedValues.TestFile.Name)
		resp, err := cfg.HTTP.Get(portForwardURL)
		err = testURL("GET RWO Port-Forward", portForwardURL, resp, err, cfg.MergedValues.TestFile.Contents)
		if err != nil {
			return err
		}
		return nil
	})
}

func TestIngress(ctx context.Context, cfg *Config) error {
	ingressHostname := cfg.IngressHostname
	if ingressHostname == "" {
		ingressHostname = cfg.MergedValues.Deployment.Ingress.Hostname
	}
	ingressProtocol := "http"
	if cfg.IngressTLS || len(cfg.MergedValues.Deployment.Ingress.TLS) != 0 {
		ingressProtocol = "https"
	}
	ingressURL := fmt.Sprintf("%s://%s/rwx/%s", ingressProtocol, ingressHostname, cfg.MergedValues.TestFile.Name)
	req, err := http.NewRequest(http.MethodGet, ingressURL, nil)
	if err != nil {
		return err
	}
	if cfg.IngressHostname != "" {
		req.Host = cfg.MergedValues.Deployment.Ingress.Hostname
	}
	resp, err := cfg.HTTP.Do(req)
	err = testURL("GET RWO Ingress", ingressURL, resp, err, cfg.MergedValues.TestFile.Contents)
	if err != nil {
		return err
	}
	return nil
}

func TestNodePort(ctx context.Context, cfg *Config, statefulSetService *corev1.Service) error {
	nodePortHostname := cfg.MergedValues.StatefulSet.NodePortHostname
	nodePort := statefulSetService.Spec.Ports[0].NodePort
	if nodePort == 0 {
		return fmt.Errorf("LoadBalancer service does not have a nodePort assigned")
	}

	nodePortURL := fmt.Sprintf("http://%s:%d/rwx/%s", nodePortHostname, nodePort, cfg.MergedValues.TestFile.Name)
	resp, err := cfg.HTTP.Get(nodePortURL)
	err = testURL("GET RWX NodePort", nodePortURL, resp, err, cfg.MergedValues.TestFile.Contents)
	if err != nil {
		return err
	}
	return nil
}

func TestLoadBalancer(ctx context.Context, cfg *Config, statefulSetService *corev1.Service) error {
	statefulSetServiceIngresses := statefulSetService.Status.LoadBalancer.Ingress
	if len(statefulSetServiceIngresses) == 0 {
		return fmt.Errorf("LoadBalancer service has no ingresses")
	}
	for ix, ingress := range statefulSetServiceIngresses {
		if ingress.Hostname == "" && ingress.IP == "" {
			return fmt.Errorf("LoadBalancer servce ingress at index %d has neither a Hostname nor an IP", ix)
		}
		hostname := ingress.Hostname
		if hostname == "" {
			hostname = ingress.IP
		}

		if len(ingress.Ports) != len(statefulSetService.Spec.Ports) {
			return fmt.Errorf("LoadBalancer service ingress at index %d has %d ports instead of the expected %d", ix, len(ingress.Ports), len(statefulSetService.Spec.Ports))
		}
		ingressPortStatus := ingress.Ports[0]
		if ingressPortStatus.Error != nil {
			return fmt.Errorf("LoadBalancer service ingress at index %d reports error: %s", ix, *ingressPortStatus.Error)
		}
		port := ingressPortStatus.Port
		if port == 0 {
			return fmt.Errorf("LoadBalancer servce ingress at index %d has no port assigned", ix)
		}

		loadBalancerURL := fmt.Sprintf("http://%s:%d/rwx/%s", hostname, port, cfg.MergedValues.TestFile.Name)
		resp, err := cfg.HTTP.Get(loadBalancerURL)
		err = testURL(fmt.Sprintf("GET RWX LoadBalancer ingress index %d", ix), loadBalancerURL, resp, err, cfg.MergedValues.TestFile.Contents)
		if err != nil {
			return err
		}

		loadBalancerURL = fmt.Sprintf("http://%s:%d/rwo/%s", hostname, port, cfg.MergedValues.TestFile.Name)
		resp, err = cfg.HTTP.Post(loadBalancerURL, "application/octet-stream", bytes.NewBuffer([]byte(cfg.MergedValues.TestFile.Contents)))
		err = testURL(fmt.Sprintf("POST RWO LoadBalancer ingress index %d", ix), loadBalancerURL, resp, err, "")
		if err != nil {
			return err
		}

		loadBalancerURL = fmt.Sprintf("http://%s:%d/rwo/%s", hostname, port, cfg.MergedValues.TestFile.Name)
		resp, err = cfg.HTTP.Get(loadBalancerURL)
		err = testURL(fmt.Sprintf("GET RWO LoadBalancer ingress index %d", ix), loadBalancerURL, resp, err, cfg.MergedValues.TestFile.Contents)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestLogs(ctx context.Context, cfg *Config, k8sClient *kubernetes.Clientset, pod *corev1.Pod, dest io.Writer) error {
	logs, err := k8sClient.CoreV1().Pods(cfg.ReleaseNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return errors.Wrap(err, "Failed to start streaming pod logs")
	}
	defer logs.Close()
	_, err = io.Copy(dest, logs)
	if err != nil {
		return errors.Wrap(err, "Failed to stream logs")
	}
	return nil
}

func Test(ctx context.Context, cfg *Config) error {
	k8sClient, err := cfg.K8sClient()
	if err != nil {
		return errors.Wrap(err, "Failed to create Kubernetes client")
	}

	fullname := cfg.Fullname()

	log.Println("Finding pod to port-forward...")
	deploymentPod, err := cfg.PickDeploymentPod(ctx, k8sClient, fullname)
	if err != nil {
		return err
	}
	log.Printf("Found pod %s to port-forward...", deploymentPod.Name)

	log.Printf("Testing Port-Forwarding...")
	err = TestPortForward(ctx, cfg, deploymentPod)
	if err != nil {
		return err
	}

	log.Printf("Testing Ingress...")
	err = TestIngress(ctx, cfg)
	if err != nil {
		return err
	}

	log.Printf("Getting StatefulSet Service...")
	statefulSetService, err := cfg.GetStatefulSetService(ctx, k8sClient, fullname)
	if err != nil {
		return err
	}

	log.Print("Testing NodePort...")
	err = TestNodePort(ctx, cfg, statefulSetService)
	if err != nil {
		return err
	}

	log.Print("Testing LoadBalancer...")
	err = TestLoadBalancer(ctx, cfg, statefulSetService)
	if err != nil {
		return err
	}

	log.Print("Testing Logs...")
	err = TestLogs(ctx, cfg, k8sClient, deploymentPod, os.Stdout)
	if err != nil {
		return err
	}

	return nil
}
