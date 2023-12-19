package main_test

import (
	"context"

	"github.com/meln5674/gosh"

	"github.com/meln5674/gingk8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("K8s Smoke Test", func() {
	It("should pass against kind w/ ingress nginx, an ingress proxy, a shared local-path-provisioner, and the tautological loadbalancer", func(ctx context.Context) {
		Expect(gosh.Pipeline(
			localHelm.Helm(ctx, cluster.GetConnection(), "get", "values", "--all", "-o", "json", k8sSmokeTest.Name),
			gosh.Command("go", "run", "cmd/test/main.go").
				WithContext(ctx).
				WithParentEnvAnd(map[string]string{
					"HTTP_PROXY":  "http://localhost:1080",
					"HTTPS_PROXY": "http://localhost:1080",
					"KUBECONFIG":  cluster.GetConnection().Kubeconfig,
				}).
				WithStreams(gingk8s.GinkgoOutErr),
		).Run()).To(Succeed())
	})
})
