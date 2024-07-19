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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	jobapiv1 "github.com/fernandoocampo/job-scheduling-operator/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// nodeStatusType node status type
type nodeStatusType string

// ComputeNodeReconciler reconciles a ComputeNode object
type ComputeNodeReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

// template fields
const (
	nodeField         = ".spec.node"
	nodeFinalizerName = "node.openinnovation.ai/finalizer"
)

// node states
const (
	nodeRunning nodeStatusType = "Running"
	nodePending nodeStatusType = "Pending"
	nodeFailed  nodeStatusType = "Failed"
	emptyState  nodeStatusType = ""
)

// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computenodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computenodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=job-scheduling-operator.openinnovation.ai,resources=computenodes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbas:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// ComputeNode Controller compares the state specified by
// the ComputeNode object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/reconcile
func (r *ComputeNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("reconciling", "computeNode", req.NamespacedName)

	computeNode, err := r.getComputeNode(ctx, req.NamespacedName)
	if err != nil {
		logger.Error(err, "getting compute node object", "name", req.NamespacedName.String())

		return ctrl.Result{}, err
	}

	if computeNode == nil {
		return ctrl.Result{}, nil
	}

	if isComputeNodeUnderDeletion(computeNode) {
		err := r.deleteResources(ctx, computeNode)
		if err != nil {
			logger.Error(err, "deleting resources", "name", req.NamespacedName.String())

			return ctrl.Result{}, err
		}

		logger.Info("deleting compute node", "name", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	err = r.addFinalizer(ctx, computeNode)
	if err != nil {
		logger.Error(err, "adding finalizer", "name", req.NamespacedName.String())

		return ctrl.Result{}, err
	}

	exist, err := r.doesComputeNodeExist(ctx, computeNode)
	if err != nil {
		logger.Error(err, "checking if ComputeNode with given Node already exists", "name", req.NamespacedName.String())
		return ctrl.Result{}, nil
	}

	if exist {
		r.Recorder.Event(computeNode, "Warning", "ComputeNodeAlreadyExists", "An Update to this ComputeNode was triggered, however the node it represents was taken by another object as well")
		return ctrl.Result{}, nil
	}

	nodeStatus, err := r.getNodeStatus(ctx, computeNode)
	if err != nil {
		logger.Error(err, "getting node status", "name", computeNode.Spec.Node)

		return ctrl.Result{}, err
	}

	if nodeStatus == emptyState {
		return ctrl.Result{}, nil
	}

	logger.Info("updating computeNode state", "computeNode", req.NamespacedName, "new_state", nodeStatus)

	err = r.updateComputeNodeState(ctx, computeNode, nodeStatus)
	if err != nil {
		logger.Error(err, "updating computeNode state", "name", req.NamespacedName, "new_state", nodeStatus)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// updateComputeNodeState update computeNode object state to indicate the state of the node to which it is linked
func (r *ComputeNodeReconciler) updateComputeNodeState(ctx context.Context, computeNode *jobapiv1.ComputeNode, newState nodeStatusType) error {
	if newState == nodeStatusType("") {
		return nil
	}

	if computeNode.Status.State == newState.String() {
		return nil
	}

	computeNode.Status.State = newState.String()

	err := r.Status().Update(ctx, computeNode)
	if err != nil {
		return fmt.Errorf("unable to update computeNode state: %w", err)
	}

	return nil
}

// getNodeStatus get the node with the given name and return its current state
func (r *ComputeNodeReconciler) getNodeStatus(ctx context.Context, computeNode *jobapiv1.ComputeNode) (nodeStatusType, error) {
	node, err := r.getNode(ctx, computeNode.Spec.Node)
	if err != nil {
		return emptyState, fmt.Errorf("unable to get node state: %w", err)
	}

	if node == nil {
		return nodePending, nil
	}

	nodeStatus := getState(node)
	if isTheSameState(computeNode, nodeStatus) {
		return emptyState, nil
	}

	return nodeStatus, nil
}

func isTheSameState(computeNode *jobapiv1.ComputeNode, newState nodeStatusType) bool {
	return computeNode.Status.State == newState.String()
}

// getComputeNode get a compute node object with the given namespaced name
func (r *ComputeNodeReconciler) getComputeNode(ctx context.Context, namespacedName types.NamespacedName) (*jobapiv1.ComputeNode, error) {
	computeNode := jobapiv1.ComputeNode{}
	err := r.Get(ctx, namespacedName, &computeNode)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("unable to get computeNode: %w", err)
	}

	return &computeNode, nil
}

// getNode get a node with the given name
func (r *ComputeNodeReconciler) getNode(ctx context.Context, name string) (*corev1.Node, error) {
	node := corev1.Node{}
	err := r.Get(ctx, types.NamespacedName{Name: name}, &node)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("unable to get node: %w", err)
	}

	return &node, nil
}

// doesComputeNodeExist check if a computenode with the given node name already exists
func (r *ComputeNodeReconciler) doesComputeNodeExist(ctx context.Context, computeNode *jobapiv1.ComputeNode) (bool, error) {
	computeNodes := jobapiv1.ComputeNodeList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(nodeField, computeNode.Spec.Node),
	}
	err := r.List(ctx, &computeNodes, listOps)
	if err != nil && apierrors.IsNotFound(err) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("unable to get computeNode list: %w", err)
	}

	if len(computeNodes.Items) > 1 {
		return true, nil
	}

	return false, nil
}

func (r *ComputeNodeReconciler) deleteResources(ctx context.Context, computeNode *jobapiv1.ComputeNode) error {
	if !controllerutil.ContainsFinalizer(computeNode, nodeFinalizerName) {
		return nil
	}

	controllerutil.RemoveFinalizer(computeNode, nodeFinalizerName)

	err := r.Update(ctx, computeNode)
	if err != nil {
		return fmt.Errorf("unable to update computenode after removing finalizer: %w", err)
	}

	log.FromContext(ctx).Info("cleaning some resources before deleting compute node object", "name", computeNode.Name)

	return nil
}

// addFinalizer add finalizer to computenode object if it does not have any
func (r *ComputeNodeReconciler) addFinalizer(ctx context.Context, computeNode *jobapiv1.ComputeNode) error {
	if controllerutil.ContainsFinalizer(computeNode, nodeFinalizerName) {
		return nil
	}

	controllerutil.AddFinalizer(computeNode, nodeFinalizerName)

	err := r.Update(ctx, computeNode)
	if err != nil {
		return fmt.Errorf("unable to add finalizer: %w", err)
	}

	return nil
}

func isComputeNodeUnderDeletion(computeNode *jobapiv1.ComputeNode) bool {
	return !computeNode.ObjectMeta.DeletionTimestamp.IsZero()
}

// getState calculates ComputeNode state
func getState(node *corev1.Node) nodeStatusType {
	for _, c := range node.Status.Conditions {
		if c.Status == corev1.ConditionFalse || c.Status == corev1.ConditionUnknown {
			continue
		}

		switch c.Type {
		case corev1.NodeReady:
			return nodeRunning
		default:
			return nodePending
		}
	}

	return nodePending
}

func (n nodeStatusType) String() string {
	return string(n)
}

func (r *ComputeNodeReconciler) findObjectsForNodes(ctx context.Context, node client.Object) []reconcile.Request {
	attachedNodes := &jobapiv1.ComputeNodeList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(nodeField, node.GetName()),
	}
	err := r.List(ctx, attachedNodes, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, item := range attachedNodes.Items {
		if item.Spec.Node != node.GetName() {
			continue
		}

		if item.Status.State == getState(node.(*corev1.Node)).String() {
			return requests
		}
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      item.GetName(),
				Namespace: item.GetNamespace(),
			},
		})
		break
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ComputeNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &jobapiv1.ComputeNode{}, nodeField, func(rawObj client.Object) []string {
		// Extract the node name from the computenode Spec, if one is provided
		computeNode := rawObj.(*jobapiv1.ComputeNode)
		if computeNode.Spec.Node == "" {
			return nil
		}
		return []string{computeNode.Spec.Node}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&jobapiv1.ComputeNode{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.findObjectsForNodes),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
