package main_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/meln5674/gingk8s"
	"github.com/meln5674/gosh"
)

func TestNodeportLoadbalancer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodeportLoadbalancer Suite")
}

var _ = BeforeSuite(func() {
	gk8s = gingk8s.ForSuite(GinkgoT())
	gk8s.Options(gk8sOpts)

	nodeportLoadbalancerImageID := gk8s.ThirdPartyImage(&nodeportLoadbalancerImage)
	ingressNginxImageID := gk8s.ThirdPartyImage(&ingressNginxImage)
	localPathProvisionerImageID := gk8s.ThirdPartyImage(&localPathProvisionerImage)
	kubeIngressProxyImageID := gk8s.ThirdPartyImage(&kubeIngressProxyImage)
	k8sSmokeTestImageIDs := gk8s.CustomImages(k8sSmokeTestImages...)

	clusterID = gk8s.Cluster(&cluster, ingressNginxImageID, nodeportLoadbalancerImageID, localPathProvisionerImageID, kubeIngressProxyImageID, k8sSmokeTestImageIDs)

	nodeportLoadbalancerID := gk8s.Release(clusterID, &nodeportLoadbalancer, nodeportLoadbalancerImageID)
	ingressNginxID := gk8s.Release(clusterID, &ingressNginx, ingressNginxImageID, nodeportLoadbalancerID)
	rolloutIngressNginxID := gk8s.ClusterAction(clusterID, "Rollout ingress-nginx", gingk8s.ClusterAction(rolloutIngressNginx), ingressNginxID)
	kubeIngressProxyID := gk8s.Release(clusterID, &kubeIngressProxy, kubeIngressProxyImageID, ingressNginxID)
	sharedLocalPathProvisionerID := gk8s.Release(clusterID, &sharedLocalPathProvisioner, localPathProvisionerImageID)
	gk8s.Release(clusterID, &k8sSmokeTest, k8sSmokeTestImageIDs, nodeportLoadbalancerID, ingressNginxID, kubeIngressProxyID, sharedLocalPathProvisionerID, rolloutIngressNginxID)

	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)
	gk8s.Setup(ctx)
})

var (
	localbin     = os.Getenv("LOCALBIN")
	localKubectl = gingk8s.KubectlCommand{
		Command: []string{filepath.Join(localbin, "kubectl")},
	}
	localHelm = gingk8s.HelmCommand{
		Command: []string{filepath.Join(localbin, "helm")},
	}
	localKind = gingk8s.KindCommand{
		Command: []string{filepath.Join(localbin, "kind")},
	}

	gk8s     gingk8s.Gingk8s
	gk8sOpts = gingk8s.SuiteOpts{
		KLogFlags:      []string{"-v=6"},
		Kubectl:        &localKubectl,
		Helm:           &localHelm,
		Manifests:      &localKubectl,
		NoSuiteCleanup: os.Getenv("K8S_SMOKE_TEST_IT_DEV_MODE") != "",
		NoCacheImages:  os.Getenv("IS_CI") != "",
		NoPull:         os.Getenv("IS_CI") != "",
		NoLoadPulled:   os.Getenv("IS_CI") != "",
	}

	kindIP  string
	cluster = gingk8s.KindCluster{
		Name:           "k8s-smoke-test-it",
		KindCommand:    &localKind,
		ConfigFilePath: "./integration-test/kind.config.yaml",
		TempDir:        "./integration-test/tmp/",
	}
	clusterID gingk8s.ClusterID

	localPathProvisionerImage = gingk8s.ThirdPartyImage{
		Name: "docker.io/rancher/local-path-provisioner:v0.0.25",
	}
	sharedLocalPathProvisioner = gingk8s.HelmRelease{
		Name: "local-path-provisioner",
		Chart: &gingk8s.HelmChart{
			LocalChartInfo: gingk8s.LocalChartInfo{
				Path: "./integration-test/local-path-provisioner/deploy/chart/local-path-provisioner",
			},
		},
		Namespace:    "local-path-shared",
		ValuesFiles:  []string{"./integration-test/values.local-path-provisioner.yaml"},
		UpgradeFlags: []string{"--create-namespace"},
	}

	kubeIngressProxyImage = gingk8s.ThirdPartyImage{
		Name: "ghcr.io/meln5674/kube-ingress-proxy:v0.3.0-rc2",
	}
	kubeIngressProxy = gingk8s.HelmRelease{
		Name: "kube-ingress-proxy",
		Chart: &gingk8s.HelmChart{
			RemoteChartInfo: gingk8s.RemoteChartInfo{
				Name: "kube-ingress-proxy",
				Repo: &gingk8s.HelmRepo{
					Name: "kube-ingress-proxy",
					URL:  "https://meln5674.github.io/kube-ingress-proxy",
				},
				Version: "v0.3.0-rc2",
			},
		},
		ValuesFiles: []string{"./integration-test/values.kube-ingress-proxy.yaml"},
	}

	nodeportLoadbalancerImage = gingk8s.ThirdPartyImage{
		Name: "ghcr.io/meln5674/nodeport-loadbalancer:v0.1.0",
	}
	nodeportLoadbalancer = gingk8s.HelmRelease{
		Name: "nodeport-loadbalancer",
		Chart: &gingk8s.HelmChart{
			OCIChartInfo: gingk8s.OCIChartInfo{
				Registry: gingk8s.HelmRegistry{
					Hostname: "ghcr.io",
				},
				Repository: "meln5674/nodeport-loadbalancer/charts/nodeport-loadbalancer",
				Version:    "v0.1.0",
			},
		},

		Set: gingk8s.Object{
			"config.controller.include.hostnames":         false,
			"config.controller.include.internalIPs":       true,
			"config.controller.include.controlPlaneNodes": true,
		},
	}

	ingressNginxImage = gingk8s.ThirdPartyImage{
		Name: "registry.k8s.io/ingress-nginx/controller:v1.7.0",
	}
	ingressNginx = gingk8s.HelmRelease{
		Name: "ingress-nginx",
		Chart: &gingk8s.HelmChart{
			RemoteChartInfo: gingk8s.RemoteChartInfo{
				Repo: &gingk8s.HelmRepo{
					Name: "ingress-nginx",
					URL:  "https://kubernetes.github.io/ingress-nginx",
				},
				Name:    "ingress-nginx",
				Version: "4.6.0",
			},
		},
		ValuesFiles: []string{"./integration-test/values.ingress-nginx.yaml"},
	}

	k8sSmokeTestImages = []*gingk8s.CustomImage{
		{
			Registry:   "local.host",
			Repository: "meln5674/k8s-smoke-test/deployment",
			BuildArgs:  map[string]string{"COMPONENT": "deployment"},
		},
		{
			Registry:   "local.host",
			Repository: "meln5674/k8s-smoke-test/statefulset",
			BuildArgs:  map[string]string{"COMPONENT": "statefulset"},
		},
		{
			Registry:   "local.host",
			Repository: "meln5674/k8s-smoke-test/job",
			BuildArgs:  map[string]string{"COMPONENT": "job"},
		},
	}
	k8sSmokeTest = gingk8s.HelmRelease{
		Name: "k8s-smoke-test",
		Chart: &gingk8s.HelmChart{
			LocalChartInfo: gingk8s.LocalChartInfo{
				Path: "./deploy/helm/k8s-smoke-test",
			},
		},
		ValuesFiles: []string{"./integration-test/values.yaml"},
		Set: gingk8s.Object{
			"image.registry":               k8sSmokeTestImages[0].Registry,
			"image.repository":             k8sSmokeTestImages[0].Repository,
			"image.tag":                    gingk8s.DefaultExtraCustomImageTags[0],
			"statefulset.nodePortHostname": getKindClusterIP,
		},
	}
)

func getKindClusterIP(gk8s gingk8s.Gingk8s, ctx context.Context, cluster gingk8s.Cluster) string {
	var kindCluster *gingk8s.KindCluster
	Expect(cluster).To(BeAssignableToTypeOf(kindCluster))
	kindCluster = cluster.(*gingk8s.KindCluster)

	var kindIP string
	Expect(
		gosh.Command("docker", "inspect", fmt.Sprintf("%s-control-plane", kindCluster.Name), "-f", "{{ .NetworkSettings.Networks.kind.IPAddress }}").
			WithStreams(gosh.FuncOut(gosh.SaveString(&kindIP))).
			Run(),
	).To(Succeed())
	return strings.TrimSpace(kindIP)
}

func rolloutIngressNginx(gk8s gingk8s.Gingk8s, ctx context.Context, cluster gingk8s.Cluster) error {
	return gk8s.KubectlRollout(ctx, cluster, gingk8s.ResourceReference{
		Kind: "ds",
		Name: "ingress-nginx-controller",
	}).Run()
}
