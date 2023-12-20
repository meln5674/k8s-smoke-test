# Kubernetes Standard Feature Smoke Test

This dummy application is designed to excercise basic features that can be reasonbly expected of any Kubernetes cluster.
It is intended to demonstrate the functionality of a Kubernetes distribution or deployment tool, typically as part of an automated test.
It can also be used as a smoke test against a production cluster.

## Features Exercised

* Image pulling
* Deployments
* StatefulSets
* Jobs
* Pod-to-Pod Networking
* Pod-to-Service Networking
* CoreDNS
* Port-Forwarding
* Pod Logs
* NodePort services
* LoadBalancer services
* Dynamic ReadWriteOnce (RWO) PVCs
* Dynamic ReadWriteMany (RWX) PVCs
* Ingress

## Design

This benchmark consists of the following components:

* A RWX PVC
* A Deployment that mounts the RWX PVC
* A StatefulSet with a RWO volume template, that also mounts the RWX PVC
* A Job which mounts the RWX volume
* An Ingress which exposes the Deployment
* A LoadBalancer Service that exposes the StatefulSet
* A local CLI which orchestrates the above 

The first three components are deployed by a helm chart.

The Job is used as a post-install/post-upgrade hook, and writes a file to the RWX PVC.

The Deployment exposes a GET endpoint which reads this file from the RWX PVC, as well as a health endpoint. Each request will also make a request to the per-Pod DNS name of the StatefulSet.

The StatefulSet exposes a GET endpoint which reads this file from the RWX PVC, a POST endpoint which writes to its RWO PVC, a GET endpoint which reads from it, and a health endpoint. Each request will also make a request to the Service DNS of the Deployment.

The CLI will first deploy the helm chart, and wait for the job to complete.

Next, the deployment's pod will be fetched using the API. This pod will be port-forwarded to, and a GET request will be sent to to retrieve the file written by the job.

Next, the pods logs will be streamed to the console.

Then, it will make a GET request to the Deployment's Ingress to retrieve the file written by the job.

Next, it will make a GET request to the NodePort underlying the StatefulSet's LoadBalancer to receive the file written by the job.

Finally, it will make a POST request to the StatefulSet's LoadBalancer service to create a file, and then a GET request to the LoadBalancer endpoint (as reported by the status) to retrieve that same file.

## Running

First, deploy the smoke test components

```bash
helm upgrade --install <release name> deploy/helm/k8s-smoke-test \
    --wait \
    --namespace <namespace> \
    --set deployment.ingress.hostname=<an available ingress hostname> \
    --set deployment.ingress.className=<name of IngressClass to test> \
    --set statefulset.nodePortHostname=<a hostname or IP that NodePort services can be reached at> \
    --set persistence.rwo.storageClassName=<name of ReadWriteOnce StorageClass to test> \
    --set persistence.rwx.storageClassName=<name of ReadWriteMany StorageClass to test>
```

Then, execute the test script

```bash
helm get values --all -o json <release name> > merged-values.json
go run cmd/test/main.go \
    --merged-values merged-values.json \
    --namespace <namespace> \
    --release-name <release name> \
    <any valid flag from kubectl>
```

The test script can also be executed from Go code by importing `github.com/meln5674/k8s-smoke-test/pkg/test`
