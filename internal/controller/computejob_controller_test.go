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

package controller_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	jobschedulingoperatorv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
	"github.com/fernandoocampo/job-scheduling-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateJob(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildScheme(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeJob := jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "kind-3-worker2",
			},
			Parallelism: int32(1),
		},
		Status: jobschedulingoperatorv1.ComputeJobStatus{
			State: "Pending",
		},
	}
	expectedBatchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        "computejob-sample",
			Namespace:   "default",
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "job-service",
							Image:   "perl:5.34.0",
							Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "kind-3-worker2",
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
	}
	givenComputeJob := buildComputeJobSampleFixture("computejob-sample", "default")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeJob).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeJob).
		Build()
	reconciler := controller.ComputeJobReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computejob-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeJob := getComputeJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})
	gottenBatchJob := getBatchJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})

	assert.Equal(t, expectedComputeJob.Spec, gottenComputeJob.Spec)
	assert.Equal(t, expectedComputeJob.Status, gottenComputeJob.Status)

	assert.Equal(t, expectedBatchJob.Spec, gottenBatchJob.Spec)
	assert.Equal(t, expectedBatchJob.Status, gottenBatchJob.Status)
}

func TestDoNothing(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildScheme(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeJob := jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "kind-3-worker2",
			},
			Parallelism: int32(1),
		},
		Status: jobschedulingoperatorv1.ComputeJobStatus{
			State: "Pending",
		},
	}
	givenComputeJob := buildPendingComputeJobSampleFixture("computejob-sample", "default")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeJob).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeJob).
		Build()
	reconciler := controller.ComputeJobReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computejob-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeJob := getComputeJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})
	gottenBatchJob := getBatchJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})

	assert.Equal(t, expectedComputeJob.Spec, gottenComputeJob.Spec)
	assert.Equal(t, expectedComputeJob.Status, gottenComputeJob.Status)

	assert.Nil(t, gottenBatchJob)
}

func TestUpdateComputeJobStateToRunning(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildScheme(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeJob := jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "kind-3-worker2",
			},
			Parallelism: int32(1),
		},
		Status: jobschedulingoperatorv1.ComputeJobStatus{
			State:       "running",
			ActiveNodes: "kind-kind-3",
			StartTime:   "2024-07-14T17:04:05+02:00",
		},
	}
	existingBatchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        "computejob-sample",
			Namespace:   "default",
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "job-service",
							Image:   "perl:5.34.0",
							Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "kind-3-worker2",
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
		Status: batchv1.JobStatus{
			StartTime: getMetaV1Date(t, "2024-07-14T15:04:05Z"),
		},
	}
	existingJobPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"batch.kubernetes.io/job-name": "computejob-sample",
			},
			Annotations: make(map[string]string),
			Name:        "computejob-sample-ijhk12",
			Namespace:   "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "kind-kind-3",
		},
	}
	givenComputeJob := buildPendingComputeJobSampleFixture("computejob-sample", "default")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeJob, existingBatchJob, existingJobPod).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeJob, existingJobPod).
		Build()
	reconciler := controller.ComputeJobReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computejob-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeJob := getComputeJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})

	assert.Equal(t, expectedComputeJob.Spec, gottenComputeJob.Spec)
	assert.Equal(t, expectedComputeJob.Status, gottenComputeJob.Status)
}

func TestUpdateComputeJobStateToCompleted(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildScheme(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeJob := jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "kind-3-worker2",
			},
			Parallelism: int32(1),
		},
		Status: jobschedulingoperatorv1.ComputeJobStatus{
			State:       "completed",
			ActiveNodes: "kind-kind-3",
			StartTime:   "2024-07-16T22:00:30+02:00",
			EndTime:     "2024-07-16T22:00:37+02:00",
		},
	}
	existingBatchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        "computejob-sample",
			Namespace:   "default",
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "job-service",
							Image:   "perl:5.34.0",
							Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "kind-3-worker2",
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
		Status: batchv1.JobStatus{
			StartTime:      getMetaV1Date(t, "2024-07-16T20:00:30Z"),
			CompletionTime: getMetaV1Date(t, "2024-07-16T20:00:37Z"),
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	givenComputeJob := buildRunningComputeJobSampleFixture("computejob-sample", "default")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeJob, existingBatchJob).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeJob).
		Build()
	reconciler := controller.ComputeJobReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computejob-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeJob := getComputeJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})

	assert.Equal(t, expectedComputeJob.Spec, gottenComputeJob.Spec)
	assert.Equal(t, expectedComputeJob.Status, gottenComputeJob.Status)
}

func TestUpdateComputeJobStateToFailed(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildScheme(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeJob := jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "kind-3-worker2",
			},
			Parallelism: int32(1),
		},
		Status: jobschedulingoperatorv1.ComputeJobStatus{
			State:       "failed",
			ActiveNodes: "kind-kind-3",
			StartTime:   "2024-07-16T22:00:30+02:00",
		},
	}
	existingBatchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        "computejob-sample",
			Namespace:   "default",
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "job-service",
							Image:   "perl:5.34.0",
							Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "kind-3-worker2",
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
		Status: batchv1.JobStatus{
			StartTime: getMetaV1Date(t, "2024-07-16T20:00:30Z"),
			Conditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	givenComputeJob := buildRunningComputeJobSampleFixture("computejob-sample", "default")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeJob, existingBatchJob).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeJob).
		Build()
	reconciler := controller.ComputeJobReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computejob-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeJob := getComputeJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})

	assert.Equal(t, expectedComputeJob.Spec, gottenComputeJob.Spec)
	assert.Equal(t, expectedComputeJob.Status, gottenComputeJob.Status)
}

func TestUpdateJobDueToUndesiredState(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildScheme(t)
	expectedReconcileResult := ctrl.Result{}
	expectedBatchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        "computejob-sample",
			Namespace:   "default",
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "job-service",
							Image:   "perl:5.34.0",
							Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "kind-3-worker2",
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
	}
	existingBatchJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        "computejob-sample",
			Namespace:   "default",
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "job-service",
							Image:   "perl:5.34.0",
							Command: []string{"perlo", "-Mbignum=bpi", "-wle", "print bpi(3000)"},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": "kind-3-worker3",
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
	}

	givenComputeJob := buildRunningComputeJobSampleFixture("computejob-sample", "default")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeJob, existingBatchJob).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeJob).
		Build()
	reconciler := controller.ComputeJobReconciler{
		Client: k8sClient,
		Scheme: scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computejob-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenBatchJob := getBatchJob(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computejob-sample"})

	assert.Equal(t, expectedBatchJob.Spec, gottenBatchJob.Spec)
}

func getComputeJob(ctx context.Context, t *testing.T, client client.WithWatch, key types.NamespacedName) *jobschedulingoperatorv1.ComputeJob {
	t.Helper()

	var computeJob jobschedulingoperatorv1.ComputeJob

	err := client.Get(ctx, key, &computeJob)
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)

	return &computeJob
}

func getBatchJob(ctx context.Context, t *testing.T, client client.WithWatch, key types.NamespacedName) *batchv1.Job {
	t.Helper()

	var job batchv1.Job

	err := client.Get(ctx, key, &job)
	if errors.IsNotFound(err) {
		return nil
	}

	require.NoError(t, err)

	return &job
}

func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	err := jobschedulingoperatorv1.AddToScheme(scheme)
	require.NoErrorf(t, err, "unexpected error adding job scheduling operator v1 to scheme")

	batchv1.AddToScheme(scheme)
	require.NoErrorf(t, err, "unexpected error adding batchv1 to scheme")

	corev1.AddToScheme(scheme)
	require.NoErrorf(t, err, "unexpected error adding corev1 to scheme")

	return scheme
}

func buildComputeJobSampleFixture(name, namespace string) *jobschedulingoperatorv1.ComputeJob {
	return &jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": "kind-3-worker2",
			},
			Parallelism: int32(1),
		},
	}
}

func buildPendingComputeJobSampleFixture(name, namespace string) *jobschedulingoperatorv1.ComputeJob {
	newComputeJob := buildComputeJobSampleFixture(name, namespace)
	newComputeJob.Status = jobschedulingoperatorv1.ComputeJobStatus{
		State: "Pending",
	}

	return newComputeJob
}

func buildRunningComputeJobSampleFixture(name, namespace string) *jobschedulingoperatorv1.ComputeJob {
	newComputeJob := buildComputeJobSampleFixture(name, namespace)
	newComputeJob.Status = jobschedulingoperatorv1.ComputeJobStatus{
		State:       "Running",
		StartTime:   "2024-07-14T17:04:05+02:00",
		ActiveNodes: "kind-kind-3",
	}

	return newComputeJob
}

func getInt32Ptr(val int32) *int32 {
	return &val
}

func getMetaV1Date(t *testing.T, date string) *metav1.Time {
	t.Helper()
	parseTime, err := time.Parse(time.RFC3339, date)
	require.NoError(t, err)

	metav1Time := metav1.NewTime(parseTime)

	return &metav1Time
}
