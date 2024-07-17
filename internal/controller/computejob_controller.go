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
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	jobapiv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
)

// jobStateType computejob state type
type jobStateType string

// actionType defines the action to execute for the operator
type actionType string

// ComputeJobReconciler reconciles a ComputeJob object
type ComputeJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// computeJobState
type computeJobState struct {
	newState       jobStateType
	action         actionType
	startTime      string
	endTime        string
	availableNodes string
}

// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computejobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computejobs/finalizers,verbs=update

// States
const (
	pending   jobStateType = "Pending"
	running   jobStateType = "running"
	completed jobStateType = "completed"
	failed    jobStateType = "failed"
	suspended jobStateType = "suspended"
)

// Actions
const (
	createJob   actionType = "createJob"
	updateState actionType = "updateState"
	updateJob   actionType = "updateJob"
	doNothing   actionType = "doNothing"
)

// Keys
const (
	jobOwnerKey = ".metadata.controller"
	finalizer   = "app.openinnovation.ai/finalizer"
)

// Job Specs
const (
	containerName            = "job-service"
	containerImageName       = "perl:5.34.0"
	jobBackoffLimit    int32 = 1
)

// Common Labels
const (
	appNameKey        = "app.openinnovation.ai/name"
	appComponentKey   = "app.openinnovation.ai/component"
	appComponentValue = "job-scheduling-operator"
	jobNameKey        = "batch.kubernetes.io/job-name"
)

var (
	apiGVStr = jobapiv1.GroupVersion.String()
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ComputeJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *ComputeJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	computeJob, err := r.getComputeJob(ctx, req.NamespacedName)
	if err != nil {
		logger.Error(err, "fetching ComputeJob", "name", req.NamespacedName)

		return ctrl.Result{}, err
	}

	if computeJob == nil {
		return ctrl.Result{}, nil
	}

	computeJobNextState, err := r.getComputeJobNextState(ctx, computeJob)
	if err != nil {
		logger.Error(err, "getting compute job next step: %w", err)
		return ctrl.Result{}, err
	}

	switch computeJobNextState.action {
	case createJob:
		logger.Info("updating computejob state before creating job",
			"name", req.NamespacedName,
			"new_state", computeJobNextState.newState)
		err := r.updateJobState(ctx, req.NamespacedName, computeJobNextState)
		if err != nil {
			logger.Error(err, "updating job status in create job action", "job", req.NamespacedName, "state", pending)
			return ctrl.Result{}, err
		}

		computeJob, err := r.getComputeJob(ctx, req.NamespacedName)
		if err != nil {
			logger.Error(err, "re-fetching ComputeJob", "name", req.NamespacedName)

			return ctrl.Result{}, err
		}

		logger.Info("creating new batch job",
			"name", req.NamespacedName,
			"new_state", computeJobNextState.newState)
		err = r.createJob(ctx, computeJob)
		if err != nil {
			logger.Error(err, "creating Job", "job", req.NamespacedName)

			return ctrl.Result{}, err
		}
	case updateState:
		logger.Info("updating computejob state",
			"name", req.NamespacedName,
			"new_state", computeJobNextState.newState)
		err := r.updateJobState(ctx, req.NamespacedName, computeJobNextState)
		if err != nil {
			logger.Error(err, "updating job status in update state action", "job", req.NamespacedName, "state", pending)
			return ctrl.Result{}, err
		}
	case doNothing:
		return ctrl.Result{}, nil
	case updateJob:
		logger.Info("updating batch job state because is not the desired state",
			"name", req.NamespacedName)
		err := r.updateBatchJob(ctx, req.NamespacedName)
		if err != nil {
			logger.Error(err, "updating job due to lack of desired state", "job", req.NamespacedName, "state", pending)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ComputeJobReconciler) getComputeJobNextState(ctx context.Context, computeJob *jobapiv1.ComputeJob) (*computeJobState, error) {
	var result computeJobState

	if computeJob.Status.State == "" {
		result.newState = pending
		result.action = createJob
		return &result, nil
	}

	job, err := r.getBatchJob(ctx, types.NamespacedName{Namespace: computeJob.Namespace, Name: computeJob.Name})
	if err != nil {
		return nil, fmt.Errorf("unable to calculate next state: %w", err)
	}

	if job == nil {
		result.action = doNothing
		return &result, nil
	}

	isJobDesiredState := hasJobDesiredState(computeJob, job)
	if !isJobDesiredState {
		result.action = updateJob
		return &result, nil
	}

	if job.Status.StartTime != nil {
		result.startTime = job.Status.StartTime.Format(time.RFC3339)
	}
	if job.Status.CompletionTime != nil {
		result.endTime = job.Status.CompletionTime.Format(time.RFC3339)
	}

	switch getCurrentJobState(job) {
	case "":
		nodes, err := r.getJobNodes(ctx, job.Namespace, job.Name)
		if err != nil {
			return nil, fmt.Errorf("unable to get job nodes: %w", err)
		}

		result.availableNodes = nodes
		result.newState = running
		result.action = updateState
	case batchv1.JobFailed:
		result.newState = failed
		result.action = updateState
	case batchv1.JobComplete:
		result.newState = completed
		result.action = updateState
	default:
		result.newState = running
		result.action = updateState
	}

	return &result, nil
}

func (r *ComputeJobReconciler) getComputeJob(ctx context.Context, namespacedName types.NamespacedName) (*jobapiv1.ComputeJob, error) {
	computeJob := jobapiv1.ComputeJob{}
	err := r.Get(ctx, namespacedName, &computeJob)
	if err != nil && errors.IsNotFound(err) {
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
	if err != nil && errors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("unable to get batch job: %w", err)
	}

	return &batchJob, nil
}

func (r *ComputeJobReconciler) updateJobState(ctx context.Context, namespacedName types.NamespacedName, state *computeJobState) error {
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

	computeJob.Status.State = state.newState.String()

	if state.startTime != "" && computeJob.Status.StartTime != state.startTime {
		computeJob.Status.StartTime = state.startTime
	}

	if state.endTime != "" && computeJob.Status.EndTime != state.endTime {
		computeJob.Status.EndTime = state.endTime
	}

	if state.availableNodes != "" && computeJob.Status.ActiveNodes != state.availableNodes {
		computeJob.Status.ActiveNodes = state.availableNodes
	}

	err = r.Status().Update(ctx, computeJob)
	if err != nil {
		return fmt.Errorf("unable to update computejob state: %w", err)
	}

	return nil
}

func (r *ComputeJobReconciler) getJobNodes(ctx context.Context, namespace, jobName string) (string, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{jobNameKey: jobName}); err != nil {
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
	newJob, err := r.buildJob(computeJob)
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

// buildJob build a k8s job object
func (r *ComputeJobReconciler) buildJob(computeJob *jobapiv1.ComputeJob) (*batchv1.Job, error) {
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
							Name:    containerName,
							Image:   containerImageName,
							Command: computeJob.Spec.Command,
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector:  computeJob.Spec.NodeSelector,
				},
			},
			BackoffLimit: getInt32Ptr(jobBackoffLimit),
		},
	}

	for k, v := range computeJob.Annotations {
		newJob.Annotations[k] = v
	}
	for k, v := range computeJob.Labels {
		newJob.Labels[k] = v
	}

	newJob.Labels[appNameKey] = computeJob.Name
	newJob.Labels[appComponentKey] = appComponentValue

	if err := ctrl.SetControllerReference(computeJob, newJob, r.Scheme); err != nil {
		return nil, err
	}

	return newJob, nil
}

func getCurrentJobState(job *batchv1.Job) batchv1.JobConditionType {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return c.Type
		}
	}

	return ""
}

func hasJobDesiredState(desiredState *jobapiv1.ComputeJob, currentState *batchv1.Job) bool {
	hasDesiredState := true
	if currentState.Spec.Parallelism != nil && *currentState.Spec.Parallelism != desiredState.Spec.Parallelism {
		hasDesiredState = false
	}

	if !maps.Equal(currentState.Spec.Template.Spec.NodeSelector, desiredState.Spec.NodeSelector) {
		hasDesiredState = false
	}

	if len(currentState.Spec.Template.Spec.Containers) != 1 {
		hasDesiredState = false
	}

	if !slices.Equal(currentState.Spec.Template.Spec.Containers[0].Command, desiredState.Spec.Command) {
		hasDesiredState = false
	}

	return hasDesiredState
}

func getInt32Ptr(val int32) *int32 {
	return &val
}

func (j jobStateType) String() string {
	return string(j)
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
		if owner.APIVersion != apiGVStr || owner.Kind != "Job" {
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
