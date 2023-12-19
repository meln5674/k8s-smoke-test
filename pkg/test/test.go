package test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

func Test(ctx context.Context, httpClient *http.Client, k8sClient *kubernetes.Clientset, releaseNamespace string, releaseName string, mergedValues *MergedValues) error {
	fullname := mergedValues.FullnameOverride
	if fullname == "" {
		if strings.Contains(releaseName, "k8s-smoke-test") {
			fullname = releaseName
		} else {
			fullname = releaseName + "-k8s-smoke-test"
		}
	}

	ingressHostname := mergedValues.Deployment.Ingress.Hostname
	ingressProtocol := "http"
	if len(mergedValues.Deployment.Ingress.TLS) != 0 {
		ingressProtocol = "https"
	}
	ingressURL := fmt.Sprintf("%s://%s/rwx/%s", ingressProtocol, ingressHostname, mergedValues.TestFile.Name)
	resp, err := http.Get(ingressURL)
	err = testURL("GET RWO Ingress", ingressURL, resp, err, mergedValues.TestFile.Contents)
	if err != nil {
		return err
	}

	loadBalancer, err := k8sClient.CoreV1().Services(releaseNamespace).Get(ctx, fullname+"-statefulset", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("LoadBalancer service does not exist: %s", err)
	}

	nodePortHostname := mergedValues.StatefulSet.NodePortHostname
	nodePort := loadBalancer.Spec.Ports[0].NodePort
	if nodePort == 0 {
		return fmt.Errorf("LoadBalancer service does not have a nodePort assigned")
	}

	nodePortURL := fmt.Sprintf("http://%s:%d/rwx/%s", nodePortHostname, nodePort, mergedValues.TestFile.Name)
	resp, err = httpClient.Get(nodePortURL)
	err = testURL("GET RWX NodePort", nodePortURL, resp, err, mergedValues.TestFile.Contents)
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

		loadBalancerURL := fmt.Sprintf("http://%s:%d/rwx/%s", hostname, port, mergedValues.TestFile.Name)
		resp, err = httpClient.Get(loadBalancerURL)
		err = testURL(fmt.Sprintf("GET RWX LoadBalancer ingress index %d", ix), loadBalancerURL, resp, err, mergedValues.TestFile.Contents)
		if err != nil {
			return err
		}

		loadBalancerURL = fmt.Sprintf("http://%s:%d/rwo/%s", hostname, port, mergedValues.TestFile.Name)
		resp, err = httpClient.Post(loadBalancerURL, "application/octet-stream", bytes.NewBuffer([]byte(mergedValues.TestFile.Contents)))
		err = testURL(fmt.Sprintf("POST RWO LoadBalancer ingress index %d", ix), loadBalancerURL, resp, err, "")
		if err != nil {
			return err
		}

		loadBalancerURL = fmt.Sprintf("http://%s:%d/rwo/%s", hostname, port, mergedValues.TestFile.Name)
		resp, err = httpClient.Get(loadBalancerURL)
		err = testURL(fmt.Sprintf("GET RWO LoadBalancer ingress index %d", ix), loadBalancerURL, resp, err, mergedValues.TestFile.Contents)
		if err != nil {
			return err
		}
	}

	return nil
}
