# job-scheduling-operator
This operator is responsible for scheduling batch jobs on existing declared nodes.

## Description
This project simulates a distributed job scheduling system. The operator should manage
compute nodes as Kubernetes Custom resource definition and handle the lifecycle of nodes and jobs within the system.

## Design and Implementation

### Design

### How it works
* This operator defines 2 controllers to handle node and batch jobs lifecycle. These controllers are ComputeNode and ComputeJob.
* The ComputeNode controller simply references an existing cluster node. This controller looks for a node referenced by ComputeNode to update its own state.
* The ComputeJob controller creates batch jobs based on the desired state declared in the ComputeJob custom resources. It assigns a node or a set of them to the batch job using Node Affinity based on the given CPU demand and the specific node name.

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Test this project

* To run unit tests.

```sh
make test
```

* To run end-to-end tests.

Make sure you are connected to your k8s cluster for testing. You can use a local k8s cluster using [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)

```sh
make test-e2e
```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/job-scheduling-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/job-scheduling-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/job-scheduling-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/job-scheduling-operator/<tag or branch>/dist/install.yaml
```

## Make command

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

