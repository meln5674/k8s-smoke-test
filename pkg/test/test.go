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

type MergedValues struct {
	FullnameOverride string            `json:"fullnameOverride"`
	TestFile         TestFile          `json:"testFile"`
	Deployment       DeploymentValues  `json:"deployment"`
	StatefulSet      StatefulSetValues `json:"statefulset"`
}

type TestFile struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

type DeploymentValues struct {
	Ingress DeploymentIngressValues `json:"ingress"`
}

type DeploymentIngressValues struct {
	Hostname string                       `json:"hostname"`
	TLS      []DeploymentIngressTLSValues `json:"tls"`
}

type DeploymentIngressTLSValues struct {
}

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

type Config struct {
	HTTP                 *http.Client
	K8sConfig            *rest.Config
	ReleaseNamespace     string
	ReleaseName          string
	MergedValues         *MergedValues
	PortForwardLocalPort int
}

func Test(ctx context.Context, cfg *Config) error {
	k8sClient, err := kubernetes.NewForConfig(cfg.K8sConfig)
	if err != nil {
		return errors.Wrap(err, "Failed to create Kubernetes client")
	}

	fullname := cfg.MergedValues.FullnameOverride
	if fullname == "" {
		if strings.Contains(cfg.ReleaseName, "k8s-smoke-test") {
			fullname = cfg.ReleaseName
		} else {
			fullname = cfg.ReleaseName + "-k8s-smoke-test"
		}
	}

	log.Println("Finding pod to port-forward...")
	deploymentService, err := k8sClient.CoreV1().Services(cfg.ReleaseNamespace).Get(ctx, fmt.Sprintf("%s-deployment", fullname), metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "Failed to get deployment service")
	}
	deploymentPods, err := k8sClient.CoreV1().Pods(cfg.ReleaseNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(deploymentService.Spec.Selector),
	})
	if err != nil {
		return errors.Wrap(err, "Failed to list deployment pods")
	}
	if len(deploymentPods.Items) == 0 {
		return errors.New("No deployment pods were present")
	}
	pod := deploymentPods.Items[0]

	log.Printf("Found pod %s to port-forward...", pod.Name)

	err = portForward(ctx, cfg.K8sConfig, cfg.ReleaseNamespace, pod.Name, []string{fmt.Sprintf("%d:8080", cfg.PortForwardLocalPort)}, func() error {
		portForwardURL := fmt.Sprintf("http://localhost:%d/rwx/%s", cfg.PortForwardLocalPort, cfg.MergedValues.TestFile.Name)
		resp, err := http.Get(portForwardURL)
		err = testURL("GET RWO Port-Forward", portForwardURL, resp, err, cfg.MergedValues.TestFile.Contents)
		if err != nil {
			return err
		}
		return nil
	})

	ingressHostname := cfg.MergedValues.Deployment.Ingress.Hostname
	ingressProtocol := "http"
	if len(cfg.MergedValues.Deployment.Ingress.TLS) != 0 {
		ingressProtocol = "https"
	}
	ingressURL := fmt.Sprintf("%s://%s/rwx/%s", ingressProtocol, ingressHostname, cfg.MergedValues.TestFile.Name)
	resp, err := http.Get(ingressURL)
	err = testURL("GET RWO Ingress", ingressURL, resp, err, cfg.MergedValues.TestFile.Contents)
	if err != nil {
		return err
	}

	loadBalancer, err := k8sClient.CoreV1().Services(cfg.ReleaseNamespace).Get(ctx, fullname+"-statefulset", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("LoadBalancer service does not exist: %s", err)
	}

	nodePortHostname := cfg.MergedValues.StatefulSet.NodePortHostname
	nodePort := loadBalancer.Spec.Ports[0].NodePort
	if nodePort == 0 {
		return fmt.Errorf("LoadBalancer service does not have a nodePort assigned")
	}

	nodePortURL := fmt.Sprintf("http://%s:%d/rwx/%s", nodePortHostname, nodePort, cfg.MergedValues.TestFile.Name)
	resp, err = cfg.HTTP.Get(nodePortURL)
	err = testURL("GET RWX NodePort", nodePortURL, resp, err, cfg.MergedValues.TestFile.Contents)
	if err != nil {
		return err
	}

	loadBalancerIngresses := loadBalancer.Status.LoadBalancer.Ingress
	if len(loadBalancerIngresses) == 0 {
		return fmt.Errorf("LoadBalancer service has no ingresses")
	}
	for ix, ingress := range loadBalancerIngresses {
		if ingress.Hostname == "" && ingress.IP == "" {
			return fmt.Errorf("LoadBalancer servce ingress at index %d has neither a Hostname nor an IP", ix)
		}
		hostname := ingress.Hostname
		if hostname == "" {
			hostname = ingress.IP
		}

		if len(ingress.Ports) != len(loadBalancer.Spec.Ports) {
			return fmt.Errorf("LoadBalancer service ingress at index %d has %d ports instead of the expected %d", ix, len(ingress.Ports), len(loadBalancer.Spec.Ports))
		}
		ingressPortStatus := ingress.Ports[0]
		if ingressPortStatus.Error != nil {
			return fmt.Errorf("LoadBalancer service ingress at index %d reports error: %s", ix, err)
		}
		port := ingressPortStatus.Port
		if port == 0 {
			return fmt.Errorf("LoadBalancer servce ingress at index %d has no port assigned", ix)
		}

		loadBalancerURL := fmt.Sprintf("http://%s:%d/rwx/%s", hostname, port, cfg.MergedValues.TestFile.Name)
		resp, err = cfg.HTTP.Get(loadBalancerURL)
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
