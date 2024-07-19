/*
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
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	jobapiv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
	"github.com/fernandoocampo/job-scheduling-operator/internal/jobs"
	"github.com/go-logr/logr"
)

// ComputeJobReconciler reconciles a ComputeJob object
type ComputeJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	logger *logr.Logger
}

// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computejobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computejobs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

// Keys
const (
	jobOwnerKey  = ".metadata.controller"
	nodeStateKey = ".status.state"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// ComputeJob controller compares the state specified by
// the ComputeJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *ComputeJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	r.logger = &logger

	computeJob, err := r.getComputeJob(ctx, req.NamespacedName)
	if err != nil {
		r.logger.Error(err, "fetching ComputeJob", "name", req.NamespacedName)

		return ctrl.Result{}, err
	}

	if computeJob == nil {
		return ctrl.Result{}, nil
	}

	computeJobNextState, err := r.getComputeJobNextState(ctx, computeJob)
	if err != nil {
		r.logger.Error(err, "getting compute job next step: %w", err)
		return ctrl.Result{}, err
	}

	switch computeJobNextState.Action {
	case jobs.CreateJobAction:
		r.logger.Info("creating job", "name", req.NamespacedName, "new_state", computeJobNextState.NewState)
		err := r.doCreateJob(ctx, req, computeJobNextState)
		if err != nil {
			r.logger.Error(err, "doing creationg job action", "computeJob", req.NamespacedName)

			return ctrl.Result{}, err
		}
	case jobs.UpdateStateAction:
		r.logger.Info("updating computejob state",
			"name", req.NamespacedName,
			"new_state", computeJobNextState.NewState)
		err := r.updateJobState(ctx, req.NamespacedName, computeJobNextState)
		if err != nil {
			r.logger.Error(err, "updating job status in update state action", "job", req.NamespacedName, "state", jobs.PendingState)
			return ctrl.Result{}, err
		}
	case jobs.DoNothingAction:
		return ctrl.Result{}, nil
	case jobs.UpdateJobAction:
		r.logger.Info("updating batch job state because is not the desired state",
			"name", req.NamespacedName)
		err := r.updateBatchJob(ctx, req.NamespacedName)
		if err != nil {
			r.logger.Error(err, "updating job due to lack of desired state", "job", req.NamespacedName, "state", jobs.PendingState)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ComputeJobReconciler) doCreateJob(ctx context.Context, req ctrl.Request, computeJobNextState *jobs.ComputeJobState) error {
	r.logger.Info("updating computejob state before creating job",
		"name", req.NamespacedName,
		"new_state", computeJobNextState.NewState)
	err := r.updateJobState(ctx, req.NamespacedName, computeJobNextState)
	if err != nil {
		return fmt.Errorf("unable to update job state: %w", err)
	}

	computeJob, err := r.getComputeJob(ctx, req.NamespacedName)
	if err != nil {
		return fmt.Errorf("unable re-fetch computejob: %w", err)
	}

	err = r.createJob(ctx, computeJob)
	if err != nil {
		return fmt.Errorf("unable to create job: %w", err)
	}

	return nil
}

func (r *ComputeJobReconciler) getComputeJobNextState(ctx context.Context, computeJob *jobapiv1.ComputeJob) (*jobs.ComputeJobState, error) {
	var result jobs.ComputeJobState

	if computeJob.Status.State == "" {
		result.NewState = jobs.PendingState
		result.Action = jobs.CreateJobAction
		return &result, nil
	}

	job, err := r.getBatchJob(ctx, types.NamespacedName{Namespace: computeJob.Namespace, Name: computeJob.Name})
	if err != nil {
		return nil, fmt.Errorf("unable to calculate next state: %w", err)
	}

	if job == nil {
		result.Action = jobs.DoNothingAction
		return &result, nil
	}

	isJobDesiredState := jobs.HasJobDesiredState(computeJob, job)
	if !isJobDesiredState {
		result.Action = jobs.UpdateJobAction
		return &result, nil
	}

	if job.Status.StartTime != nil {
		result.StartTime = job.Status.StartTime.Format(time.RFC3339)
	}
	if job.Status.CompletionTime != nil {
		result.EndTime = job.Status.CompletionTime.Format(time.RFC3339)
	}

	switch jobs.GetCurrentJobState(job) {
	case "":
		nodes, err := r.getJobNodes(ctx, job.Namespace, job.Name)
		if err != nil {
			return nil, fmt.Errorf("unable to get job nodes: %w", err)
		}

		result.AvailableNodes = nodes
		result.NewState = jobs.RunningState
		result.Action = jobs.UpdateStateAction
	case batchv1.JobFailed:
		result.NewState = jobs.FailedState
		result.Action = jobs.UpdateStateAction
	case batchv1.JobComplete:
		result.NewState = jobs.CompletedState
		result.Action = jobs.UpdateStateAction
	default:
		result.NewState = jobs.RunningState
		result.Action = jobs.UpdateStateAction
	}

	return &result, nil
}

func (r *ComputeJobReconciler) getComputeJob(ctx context.Context, namespacedName types.NamespacedName) (*jobapiv1.ComputeJob, error) {
	computeJob := jobapiv1.ComputeJob{}
	err := r.Get(ctx, namespacedName, &computeJob)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("unable to get ComputeJob: %w", err)
	}

	return &computeJob, nil
}

func (r *ComputeJobReconciler) getBatchJob(ctx context.Context, namespacedName types.NamespacedName) (*batchv1.Job, error) {
	batchJob := batchv1.Job{}
	err := r.Get(ctx, namespacedName, &batchJob)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("unable to get batch job: %w", err)
	}

	return &batchJob, nil
}

func (r *ComputeJobReconciler) updateJobState(ctx context.Context, namespacedName types.NamespacedName, state *jobs.ComputeJobState) error {
	if state == nil {
		return nil
	}

	computeJob, err := r.getComputeJob(ctx, namespacedName)
	if err != nil {
		return fmt.Errorf("unable to update computejob state: %w", err)
	}

	if computeJob == nil {
		return nil
	}

	computeJob.Status.State = state.NewState.String()

	if state.StartTime != "" && computeJob.Status.StartTime != state.StartTime {
		computeJob.Status.StartTime = state.StartTime
	}

	if state.EndTime != "" && computeJob.Status.EndTime != state.EndTime {
		computeJob.Status.EndTime = state.EndTime
	}

	if state.AvailableNodes != "" && computeJob.Status.ActiveNodes != state.AvailableNodes {
		computeJob.Status.ActiveNodes = state.AvailableNodes
	}

	err = r.Status().Update(ctx, computeJob)
	if err != nil {
		return fmt.Errorf("unable to update computejob state: %w", err)
	}

	return nil
}

func (r *ComputeJobReconciler) getJobNodes(ctx context.Context, namespace, jobName string) (string, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{jobs.JobNameKey: jobName}); err != nil {
		return "", fmt.Errorf("unable to list job pods: %w", err)
	}

	var nodes []string
	for _, pod := range pods.Items {
		nodes = append(nodes, pod.Spec.NodeName)
	}

	var result string
	if len(nodes) > 0 {
		result = strings.Join(nodes, ",")
	}

	return result, nil
}

// createJob build and create a k8s job
func (r *ComputeJobReconciler) createJob(ctx context.Context, computeJob *jobapiv1.ComputeJob) error {
	nodesToRun, err := r.getNodesToRun(ctx, computeJob)
	if err != nil {
		return fmt.Errorf("unable to get nodes to run: %w", err)
	}
	newJob, err := jobs.BuildJob(computeJob, r.Scheme, nodesToRun)
	if err != nil {
		return fmt.Errorf("unable to build job from template: %w", err)
	}

	if err := r.Create(ctx, newJob); err != nil {
		return fmt.Errorf("unable to create job: %w", err)
	}

	return nil
}

// updateJob update job state
func (r *ComputeJobReconciler) updateBatchJob(ctx context.Context, namespacedName types.NamespacedName) error {
	batchJob, err := r.getBatchJob(ctx, namespacedName)
	if err != nil {
		return fmt.Errorf("unable to update job, couldn't get from k8s: %w", err)
	}
	if batchJob == nil {
		return nil
	}

	if err := r.Delete(ctx, batchJob, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
		return fmt.Errorf("unable to update job, couldn't delete it in k8s: %w", err)
	}

	computeJob, err := r.getComputeJob(ctx, namespacedName)
	if err != nil {
		return fmt.Errorf("unable to update job, couldn't get computejob in k8s: %w", err)
	}

	if computeJob == nil {
		return nil
	}

	computeJob.Status = jobapiv1.ComputeJobStatus{}
	if err := r.Update(ctx, computeJob); err != nil {
		return fmt.Errorf("unable to update job, couldn't update computejob in k8s: %w", err)
	}

	if err := r.createJob(ctx, computeJob); err != nil {
		return fmt.Errorf("unable to update job, couldn't create batch job in k8s: %w", err)
	}

	return nil
}

func (r *ComputeJobReconciler) getNodesToRun(ctx context.Context, computeJob *jobapiv1.ComputeJob) ([]string, error) {
	computeNodes, err := r.getComputeNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to calculate nodes to use: %w", err)
	}

	// let's set it free to choose any node (no rules about this)
	if len(computeNodes) == 0 {
		return nil, errors.New("no node has been specified as computeNode")
	}

	result, err := jobs.CalculateNodesForJob(ctx, computeJob, computeNodes)
	if err != nil {
		return nil, fmt.Errorf("unable to calculate nodes for job: %w", err)
	}

	return result, nil
}

// getComputeNode get compute node objects with running state.
func (r *ComputeJobReconciler) getComputeNodes(ctx context.Context) ([]jobs.ComputeNodeResourcesItem, error) {
	computeNodes := jobapiv1.ComputeNodeList{}

	err := r.List(ctx, &computeNodes)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("unable to get computeNode list: %w", err)
	}

	result := make([]jobs.ComputeNodeResourcesItem, len(computeNodes.Items))

	for i, computeNode := range computeNodes.Items {
		if computeNode.Status.State != nodeRunning.String() {
			continue
		}
		result[i] = jobs.ComputeNodeResourcesItem{
			Name:   computeNode.Spec.Node,
			CPU:    computeNode.Spec.Resources.CPU,
			Memory: computeNode.Spec.Resources.Memory,
		}
	}

	fmt.Println("result", result)

	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComputeJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &batchv1.Job{}, jobOwnerKey, func(rawObj client.Object) []string {
		// grab the job object, extract the owner...
		job := rawObj.(*batchv1.Job)
		owner := metav1.GetControllerOf(job)
		if owner == nil {
			return nil
		}
		// ...make sure it's a Job...
		if owner.APIVersion != jobapiv1.GroupVersion.String() || owner.Kind != "Job" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&jobapiv1.ComputeJob{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
