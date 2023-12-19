package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	flag "github.com/spf13/pflag"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/meln5674/k8s-smoke-test/pkg/test"
)

var (
	releaseName         = flag.String("release-name", "k8s-smoke-test", "Name of the release")
	mergedValuesPath    = flag.String("merged-values-json", "-", "Path to the merged helm values, in JSON format, or `-` for STDIN")
	kubeconfig          = flag.String("kubeconfig", os.Getenv("KUBECONFIG"), "Path to the kubeconfig file to use for CLI requests.")
	kubernetesOverrides clientcmd.ConfigOverrides
)

func main() {
	ctx := context.Background()
	clientcmd.BindOverrideFlags(&kubernetesOverrides, flag.CommandLine, clientcmd.RecommendedConfigOverrideFlags(""))
	var err error
	mergedValuesStream := os.Stdin
	if *mergedValuesPath != "-" {
		mergedValuesStream, err = os.Open(*mergedValuesPath)
		if err != nil {
			log.Fatal(err)
		}
		defer mergedValuesStream.Close()
	}
	var mergedValues test.MergedValues
	err = json.NewDecoder(mergedValuesStream).Decode(&mergedValues)
	if err != nil {
		log.Fatal(err)
	}

	clientConfigLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{
			ExplicitPath: *kubeconfig,
		},
		&kubernetesOverrides,
	)

	namespace, _, err := clientConfigLoader.Namespace()
	if err != nil {
		log.Fatal(err)
	}

	clientConfig, err := clientConfigLoader.ClientConfig()
	if err != nil {
		log.Fatal(err)
	}

	k8sClient, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		log.Fatal(err)
	}

	err = test.Test(ctx, http.DefaultClient, k8sClient, namespace, *releaseName, &mergedValues)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("PASSED")
}
