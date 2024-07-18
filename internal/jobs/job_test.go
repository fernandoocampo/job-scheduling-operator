package jobs_test

import (
	"context"
	"errors"
	"testing"

	jobschedulingoperatorv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
	"github.com/fernandoocampo/job-scheduling-operator/internal/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildJob(t *testing.T) {
	t.Parallel()
	// Given
	scheme := buildComputeJobControllerSchemes(t)
	nodes := []string{"kind-3-worker2"}
	givenComputeJob := jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: &jobschedulingoperatorv1.NodeSelector{
				NodeName: getStringPtr("kind-3-worker2"),
			},
			Parallelism: int32(1),
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
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/hostname",
												Operator: corev1.NodeSelectorOpIn,
												Values:   []string{"kind-3-worker2"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			BackoffLimit: getInt32Ptr(1),
		},
	}

	// When
	got, err := jobs.BuildJob(&givenComputeJob, scheme, nodes)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedBatchJob.Spec, got.Spec)
}

func TestHasJobDesiredState(t *testing.T) {
	// Given
	givenComputeJob := &jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computejob-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
			NodeSelector: &jobschedulingoperatorv1.NodeSelector{
				NodeName: getStringPtr("kind-3-worker2"),
			},
			Parallelism: int32(1),
		},
	}

	testCases := []struct {
		name          string
		givenBatchJob jobFixture
		want          bool
	}{
		{
			name: "valid_state",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       1,
			},
			want: true,
		},
		{
			name: "invalid_state_parallelism",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       2,
			},
			want: false,
		},
		{
			name: "invalid_state_commands",
			givenBatchJob: jobFixture{
				containerCommands: []string{"python", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       2,
			},
			want: false,
		},
		{
			name: "invalid_state_containers",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       1,
				containers:        2,
			},
			want: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			got := jobs.HasJobDesiredState(givenComputeJob, buildJobFixture(testCase.givenBatchJob))
			// Then
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestGetCurrentJobState(t *testing.T) {
	// Given
	testCases := []struct {
		name          string
		givenBatchJob jobFixture
		want          batchv1.JobConditionType
	}{
		{
			name: "completed",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       1,
				conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
				},
			},
			want: batchv1.JobComplete,
		},
		{
			name: "failed",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       1,
				conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionTrue,
					},
				},
			},
			want: batchv1.JobFailed,
		},
		{
			name: "empty",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       1,
				conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobSuspended,
						Status: corev1.ConditionTrue,
					},
				},
			},
			want: batchv1.JobConditionType(""),
		},
		{
			name: "completed_with_multiple",
			givenBatchJob: jobFixture{
				containerCommands: []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				nodeNames:         []string{"kind-3-worker2"},
				name:              "computejob-sample",
				namespace:         "default",
				containerName:     "job-service",
				containerImage:    "perl:5.34.0",
				parallelism:       1,
				conditions: []batchv1.JobCondition{
					{
						Type:   batchv1.JobComplete,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   batchv1.JobFailed,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   batchv1.JobSuspended,
						Status: corev1.ConditionFalse,
					},
				},
			},
			want: batchv1.JobComplete,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			got := jobs.GetCurrentJobState(buildJobFixture(testCase.givenBatchJob))
			// Then
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestCalculateNodesForJob(t *testing.T) {
	// Given
	testCases := []struct {
		name            string
		ctx             context.Context
		givenComputeJob computeJobFixture
		computeNodes    []jobs.ComputeNodeResourcesItem
		isError         bool
		want            []string
		wantedError     error
	}{
		{
			name: "not_enough_nodes",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(3),
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker2",
					CPU:  8,
				},
				{
					Name: "kind-3-worker1",
					CPU:  4,
				},
			},
			isError:     true,
			wantedError: errors.New("unable to create node selector because parallelism (3) is greater than the number of computeNodes (2)"),
		},
		{
			name: "not_enough_nodes_with_required_cpu",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(3),
				nodeSelector: &jobschedulingoperatorv1.NodeSelector{
					CPU: getInt32Ptr(5),
				},
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker2",
					CPU:  8,
				},
				{
					Name: "kind-3-worker1",
					CPU:  4,
				},
				{
					Name: "kind-3-worker1",
					CPU:  5,
				},
			},
			isError:     true,
			wantedError: errors.New("unable to create node selector because parallelism (3) is greater than the number of computeNodes with enough CPU (2)"),
		},
		{
			name: "nil_node_selector",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(1),
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker2",
					CPU:  8,
				},
				{
					Name: "kind-3-worker1",
					CPU:  4,
				},
			},
			want: []string{"kind-3-worker2"},
		},
		{
			name: "specific_node",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(1),
				nodeSelector: &jobschedulingoperatorv1.NodeSelector{
					NodeName: getStringPtr("kind-3-worker1"),
				},
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker3",
					CPU:  8,
				},
				{
					Name: "kind-3-worker2",
					CPU:  4,
				},
				{
					Name: "kind-3-worker1",
					CPU:  2,
				},
			},
			want: []string{"kind-3-worker1"},
		},
		{
			name: "specific_node",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(1),
				nodeSelector: &jobschedulingoperatorv1.NodeSelector{
					NodeName: getStringPtr("kind-3-worker1"),
				},
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker3",
					CPU:  8,
				},
				{
					Name: "kind-3-worker2",
					CPU:  4,
				},
				{
					Name: "kind-3-worker1",
					CPU:  2,
				},
			},
			want: []string{"kind-3-worker1"},
		},
		{
			name: "specific_node_dont_exist",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(1),
				nodeSelector: &jobschedulingoperatorv1.NodeSelector{
					NodeName: getStringPtr("kind-3-worker-n"),
				},
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker3",
					CPU:  8,
				},
				{
					Name: "kind-3-worker2",
					CPU:  4,
				},
				{
					Name: "kind-3-worker1",
					CPU:  2,
				},
			},
			want: []string{"kind-3-worker3"},
		},
		{
			name: "look_for_cpus_parallel_1",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(1),
				nodeSelector: &jobschedulingoperatorv1.NodeSelector{
					CPU: getInt32Ptr(2),
				},
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker3",
					CPU:  8,
				},
				{
					Name: "kind-3-worker2",
					CPU:  4,
				},
				{
					Name: "kind-3-worker1",
					CPU:  2,
				},
			},
			want: []string{"kind-3-worker3"},
		},
		{
			name: "look_for_cpus_parallel_2",
			ctx:  context.TODO(),
			givenComputeJob: computeJobFixture{
				name:        "computejob-sample",
				namespace:   "default",
				command:     []string{"perl", "-Mbignum=bpi", "-wle", "print bpi(2000)"},
				parallelism: int32(2),
				nodeSelector: &jobschedulingoperatorv1.NodeSelector{
					CPU: getInt32Ptr(2),
				},
			},
			computeNodes: []jobs.ComputeNodeResourcesItem{
				{
					Name: "kind-3-worker3",
					CPU:  8,
				},
				{
					Name: "kind-3-worker2",
					CPU:  4,
				},
				{
					Name: "kind-3-worker1",
					CPU:  2,
				},
			},
			want: []string{"kind-3-worker3", "kind-3-worker2"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			got, err := jobs.CalculateNodesForJob(testCase.ctx, buildComputeJob(testCase.givenComputeJob), testCase.computeNodes)
			// Then
			if !testCase.isError {
				require.NoError(t, err)
			}
			if testCase.isError {
				assert.Equal(t, testCase.wantedError, err)
			}
			assert.Equal(t, testCase.want, got)
		})
	}
}

type jobFixture struct {
	conditions                                     []batchv1.JobCondition
	containerCommands, nodeNames                   []string
	name, namespace, containerName, containerImage string
	parallelism                                    int32
	containers                                     int32
}

type computeJobFixture struct {
	nodeSelector *jobschedulingoperatorv1.NodeSelector
	command      []string
	name         string
	namespace    string
	parallelism  int32
}

func buildJobFixture(params jobFixture) *batchv1.Job {
	if params.containers == 0 || params.containers < 0 {
		params.containers = 1
	}
	containers := make([]corev1.Container, params.containers)
	for i := range params.containers {
		containers[i] = corev1.Container{
			Name:    params.containerName,
			Image:   params.containerImage,
			Command: params.containerCommands,
		}
	}
	newJob := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        params.name,
			Namespace:   params.namespace,
		},
		Spec: batchv1.JobSpec{
			Parallelism: getInt32Ptr(params.parallelism),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:    containers,
					RestartPolicy: corev1.RestartPolicyNever,
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/hostname",
												Operator: corev1.NodeSelectorOpIn,
												Values:   params.nodeNames,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Status: batchv1.JobStatus{
			Conditions: params.conditions,
		},
	}

	return &newJob
}

func buildComputeJob(params computeJobFixture) *jobschedulingoperatorv1.ComputeJob {
	return &jobschedulingoperatorv1.ComputeJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.name,
			Namespace: params.namespace,
		},
		Spec: jobschedulingoperatorv1.ComputeJobSpec{
			Command:      params.command,
			NodeSelector: params.nodeSelector,
			Parallelism:  params.parallelism,
		},
	}
}

func buildComputeJobControllerSchemes(t *testing.T) *runtime.Scheme {
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

func getStringPtr(val string) *string {
	return &val
}

func getInt32Ptr(val int32) *int32 {
	return &val
}
