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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	jobschedulingoperatorv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
	"github.com/fernandoocampo/job-scheduling-operator/internal/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/record"
)

func TestUpdateComputeNodeStateToRunning(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildComputeNodeControllerSchemes(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeNode := jobschedulingoperatorv1.ComputeNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computenode-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeNodeSpec{
			Node: "kind-2-worker",
			Resources: jobschedulingoperatorv1.Resources{
				CPU:    8,
				Memory: 6060,
			},
		},
		Status: jobschedulingoperatorv1.ComputeNodeStatus{
			State: "Running",
		},
	}
	existingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"kubernetes.io/hostname": "kind-2-worker",
			},
			Annotations: make(map[string]string),
			Name:        "kind-2-worker",
		},
		Spec: corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Status: corev1.ConditionTrue,
					Type:   corev1.NodeReady,
				},
			},
		},
	}
	givenComputeNode := &jobschedulingoperatorv1.ComputeNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computenode-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeNodeSpec{
			Node: "kind-2-worker",
			Resources: jobschedulingoperatorv1.Resources{
				CPU:    8,
				Memory: 6060,
			},
		},
		Status: jobschedulingoperatorv1.ComputeNodeStatus{
			State: "pending",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeNode, existingNode).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeNode).
		WithIndex(givenComputeNode, ".spec.node", makeExtractValueFunc()).
		Build()
	reconciler := controller.ComputeNodeReconciler{
		Client:   k8sClient,
		Recorder: newEventRecorderMock(),
		Scheme:   scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computenode-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeNode := getComputeNode(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computenode-sample"})

	assert.NotNil(t, gottenComputeNode)
	assert.Equal(t, expectedComputeNode.Spec, gottenComputeNode.Spec)
	assert.Equal(t, expectedComputeNode.Status, gottenComputeNode.Status)
}

func TestUpdateComputeNodeStateToPending(t *testing.T) {
	t.Parallel()
	// Given
	ctrl.SetLogger(zap.New())
	scheme := buildComputeNodeControllerSchemes(t)
	expectedReconcileResult := ctrl.Result{}
	expectedComputeNode := jobschedulingoperatorv1.ComputeNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computenode-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeNodeSpec{
			Node: "kind-2-worker",
			Resources: jobschedulingoperatorv1.Resources{
				CPU:    8,
				Memory: 6060,
			},
		},
		Status: jobschedulingoperatorv1.ComputeNodeStatus{
			State: "Pending",
		},
	}
	existingNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"kubernetes.io/hostname": "kind-2-worker",
			},
			Annotations: make(map[string]string),
			Name:        "kind-2-worker",
		},
		Spec: corev1.NodeSpec{},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Status: corev1.ConditionFalse,
					Type:   corev1.NodeReady,
				},
			},
		},
	}
	givenComputeNode := &jobschedulingoperatorv1.ComputeNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "computenode-sample",
			Namespace: "default",
		},
		Spec: jobschedulingoperatorv1.ComputeNodeSpec{
			Node: "kind-2-worker",
			Resources: jobschedulingoperatorv1.Resources{
				CPU:    8,
				Memory: 6060,
			},
		},
		Status: jobschedulingoperatorv1.ComputeNodeStatus{
			State: "running",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(givenComputeNode, existingNode).
		WithScheme(scheme).
		WithStatusSubresource(givenComputeNode).
		WithIndex(givenComputeNode, ".spec.node", makeExtractValueFunc()).
		Build()
	reconciler := controller.ComputeNodeReconciler{
		Client:   k8sClient,
		Recorder: newEventRecorderMock(),
		Scheme:   scheme,
	}
	ctx := context.TODO()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "computenode-sample",
		},
	}
	// When
	got, err := reconciler.Reconcile(ctx, req)

	// Then
	require.NoError(t, err)
	assert.Equal(t, expectedReconcileResult, got)

	gottenComputeNode := getComputeNode(ctx, t, k8sClient, types.NamespacedName{Namespace: "default", Name: "computenode-sample"})

	assert.NotNil(t, gottenComputeNode)
	assert.Equal(t, expectedComputeNode.Spec, gottenComputeNode.Spec)
	assert.Equal(t, expectedComputeNode.Status, gottenComputeNode.Status)
}

func buildComputeNodeControllerSchemes(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	err := jobschedulingoperatorv1.AddToScheme(scheme)
	require.NoErrorf(t, err, "unexpected error adding job scheduling operator v1 to scheme")

	corev1.AddToScheme(scheme)
	require.NoErrorf(t, err, "unexpected error adding corev1 to scheme")

	return scheme
}

func getComputeNode(ctx context.Context, t *testing.T, client client.WithWatch, key types.NamespacedName) *jobschedulingoperatorv1.ComputeNode {
	t.Helper()

	var computeNode jobschedulingoperatorv1.ComputeNode

	err := client.Get(ctx, key, &computeNode)
	if errors.IsNotFound(err) {
		return nil
	}
	require.NoError(t, err)

	return &computeNode
}

type eventRecorderMock struct {
	record.EventRecorder
}

func newEventRecorderMock() *eventRecorderMock {
	return &eventRecorderMock{}
}

func (e *eventRecorderMock) Event(object runtime.Object, eventtype, reason, message string) {
	// Do nothing
}

func makeExtractValueFunc() func(rawObj client.Object) []string {
	return func(rawObj client.Object) []string {
		// Extract the node name from the computenode Spec, if one is provided
		computeNode := rawObj.(*jobschedulingoperatorv1.ComputeNode)
		if computeNode.Spec.Node == "" {
			return nil
		}
		return []string{computeNode.Spec.Node}
	}
}
