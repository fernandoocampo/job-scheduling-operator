# Assumptions

## Operator
1. More event notifications via recorder object will be provided in future releases.
2. CI/CD pipelines could be provided in future releases.
3. I used Kubebuilder because it is the framework in which I have the most experience.

## ComputeJob Resources
1. Batch Jobs will have same name and namespace as its ComputeJob object.
2. Batch Jobs will start immediately once they are created. They do not have a future start time.
3. Batch Jobs will run once.
4. Batch Jobs are not created as CronJobs.
5. Failed batch jobs will remain failed.
6. All batch jobs will run with the same container image hardcoded in the controller. Image is well known and it is not part of the configuration.
7. ComputeJobs and batch Jobs will be created in the same namespace as this operator.
8. Batch jobs will have a backofflimit of 1. It means just one retry.
9. Finalizers are not required because only admin can delete them, but it is an improvement in the short term.
10. If compute job desired state is changed, the batch job will be deleted and recreated.
11. If the number of ComputeNodes is less than the parallelism value in the job, the batch job won't be created.
12. For this release the node selector is only based on node name and CPU. More fields will be supported in future versions.
13. If the node selector in the compute job specify a node name, this node will be assigned to the job if it exists.
14. If there is only one compute node and the job does not specify a node selector, this node will be assigned to the batch job.
15. The node selector by CPU for ComputeJobs will select all the nodes with CPU greater than the given CPU in the job.
16. If the number of ComputeNodes with enough CPU to run the jobs is less than the parallelism value in the job, the batch job won't be created.
17. Only computenodes in state running will be selected to run the job.

## ComputeNode Resources
1. ComputeNode needs to specify the name or hostname of the node it represents. So I added a Node field in the specifications section of the ComputeNode CRD.
2. ComputeNode controller does not provision Nodes automatically. They must exist before the ComputeNode is created, otherwise ComputeNode's state will be `pending`.
3. If a ComputeNode object is created with a given node reference that is used by another existing ComputeNode resource, an event will be created to indicate that the new ComputeNode is using a node that was already registered.
4. For each ComputeNode that is created the user will provide existing CPU and memory resources. They are not updated by the operator.

## Metrics
1. Metrics are not enabled, they are out the scope of this release. Existing ones are provided by Kubebuilder.

## Tests
1. I follow unit test principle about testing only public functions to keep encapsulation.
2. For unit testing I leveraged mainly on package `"sigs.k8s.io/controller-runtime/pkg/client/fake"` which helped me to mock many external resources.
3. Kind cluster is the tool used to test this project.
4. End-To-End and Unit tests will have only happy paths. Edge cases will be provided in a future release.