package jobs

import (
	"context"
	"fmt"
	"slices"

	jobapiv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

// JobStateType computejob state type
type JobStateType string

// ActionType defines the action to execute for the computejob operator
type ActionType string

// ComputeJobState contains computejob object status
type ComputeJobState struct {
	NewState       JobStateType
	Action         ActionType
	StartTime      string
	EndTime        string
	AvailableNodes string
}

// ComputeNodeResourcesItem defines a node item with cpu and memory resources
type ComputeNodeResourcesItem struct {
	Name   string
	CPU    int32
	Memory int32
}

// States
const (
	PendingState   JobStateType = "Pending"
	RunningState   JobStateType = "running"
	CompletedState JobStateType = "completed"
	FailedState    JobStateType = "failed"
	SuspendedState JobStateType = "suspended"
)

// Actions
const (
	CreateJobAction   ActionType = "createJob"
	UpdateStateAction ActionType = "updateState"
	UpdateJobAction   ActionType = "updateJob"
	DoNothingAction   ActionType = "doNothing"
)

// Label, annotation Keys and values
const (
	JobOwnerKey       = ".metadata.controller"
	AppNameKey        = "app.openinnovation.ai/name"
	AppComponentKey   = "app.openinnovation.ai/component"
	JobNameKey        = "batch.kubernetes.io/job-name"
	NodeHostnameKey   = "kubernetes.io/hostname"
	AppComponentValue = "job-scheduling-operator"
)

// Job Specs Data
const (
	ContainerName            = "job-service"
	ContainerImageName       = "perl:5.34.0"
	JobBackoffLimit    int32 = 1
)

// Error Messages
const (
	parallelismGreaterThanNodesMessagePattern = "unable to create node selector because parallelism (%d) is greater than the number of computeNodes (%d)"
	parallelismGreaterThanNodesWithEnoughCPU  = "unable to create node selector because parallelism (%d) is greater than the number of computeNodes with enough CPU (%d)"
)

func (j JobStateType) String() string {
	return string(j)
}

// buildNodeAffinity build node affinity for job pods.
func buildNodeAffinity(nodes []string) *corev1.Affinity {
	if len(nodes) == 0 {
		return nil
	}

	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      NodeHostnameKey,
								Operator: corev1.NodeSelectorOpIn,
								Values:   nodes,
							},
						},
					},
				},
			},
		},
	}
}

// BuildJob build a k8s batch job object.
func BuildJob(computeJob *jobapiv1.ComputeJob, scheme *runtime.Scheme, nodes []string) (*batchv1.Job, error) {
	newJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        computeJob.Name,
			Namespace:   computeJob.Namespace,
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(computeJob.Spec.Parallelism),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    ContainerName,
							Image:   ContainerImageName,
							Command: computeJob.Spec.Command,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Affinity:      buildNodeAffinity(nodes),
				},
			},
			BackoffLimit: getInt32Ptr(JobBackoffLimit),
		},
	}

	for k, v := range computeJob.Annotations {
		newJob.Annotations[k] = v
	}
	for k, v := range computeJob.Labels {
		newJob.Labels[k] = v
	}

	newJob.Labels[AppNameKey] = computeJob.Name
	newJob.Labels[AppComponentKey] = AppComponentValue

	if err := ctrl.SetControllerReference(computeJob, newJob, scheme); err != nil {
		return nil, err
	}

	return newJob, nil
}

// HasJobDesiredState check if the given batch job has the desired state specified in the given computejob
func HasJobDesiredState(desiredState *jobapiv1.ComputeJob, currentState *batchv1.Job) bool {
	if currentState.Spec.Parallelism != nil && *currentState.Spec.Parallelism != desiredState.Spec.Parallelism {
		return false
	}

	if len(currentState.Spec.Template.Spec.Containers) != 1 {
		return false
	}

	if !slices.Equal(currentState.Spec.Template.Spec.Containers[0].Command, desiredState.Spec.Command) {
		return false
	}

	return true
}

// GetCurrentJobState returns the current condition of the job
func GetCurrentJobState(job *batchv1.Job) batchv1.JobConditionType {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return c.Type
		}
	}

	return ""
}

// CalculateNodesForJob based on the given computejob desired state checks what nodes the job should run
func CalculateNodesForJob(ctx context.Context, computeJob *jobapiv1.ComputeJob, computeNodes []ComputeNodeResourcesItem) ([]string, error) {
	// The job requests to parallelize more than the cluster has.
	if len(computeNodes) < int(computeJob.Spec.Parallelism) {
		return nil, fmt.Errorf(parallelismGreaterThanNodesMessagePattern, computeJob.Spec.Parallelism, len(computeNodes))
	}

	var result []string

	// no node selector or only one compute node, so let's pick any existing computeNode.
	if computeJob.Spec.NodeSelector == nil || len(computeNodes) == 1 {
		result = append(result, computeNodes[0].Name)
		return result, nil
	}

	// manifests has specified on node nade, so let's search for that node in the compute nodes
	// use it if it exists.
	if computeJob.Spec.NodeSelector.NodeName != nil &&
		*computeJob.Spec.NodeSelector.NodeName != "" &&
		computeJob.Spec.Parallelism == 1 {
		for _, computeNode := range computeNodes {
			if computeNode.Name == *computeJob.Spec.NodeSelector.NodeName {
				result = append(result, computeNode.Name)
				return result, nil
			}
		}
	}

	// no luck with node nade and no any cpu specified, let's pick any existing computeNode.
	if computeJob.Spec.NodeSelector.CPU == nil {
		result = append(result, computeNodes[0].Name)
		return result, nil
	}

	var nodesWithEnoughCPUForJob int
	for _, computeNode := range computeNodes {
		if computeNode.CPU >= *computeJob.Spec.NodeSelector.CPU {
			nodesWithEnoughCPUForJob++
		}
	}

	// cluster does not have as many nodes with enough cpus for the requested parallelism
	if nodesWithEnoughCPUForJob < int(computeJob.Spec.Parallelism) {
		return nil, fmt.Errorf(parallelismGreaterThanNodesWithEnoughCPU, computeJob.Spec.Parallelism, nodesWithEnoughCPUForJob)
	}

	var requiredInstances int32
	for _, computeNode := range computeNodes {
		if computeNode.CPU >= *computeJob.Spec.NodeSelector.CPU {
			if requiredInstances == computeJob.Spec.Parallelism {
				break
			}
			result = append(result, computeNode.Name)
			requiredInstances++
		}
	}

	return result, nil
}

func getInt32Ptr(val int32) *int32 {
	return &val
}
