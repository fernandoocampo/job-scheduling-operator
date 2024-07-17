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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Resources The resources available to the node (CPU, memory)
type Resources struct {
	// CPU available to the node in cores
	CPU int32 `json:"cpu"`
	// Memory available to the node in MB
	Memory int32 `json:"memory"`
}

// ComputeNodeSpec defines the desired state of ComputeNode
type ComputeNodeSpec struct {
	// Node name which represents this object
	Node string `json:"node"`
	// Resources available to the node
	Resources Resources `json:"resources"`
}

// ComputeNodeStatus defines the observed state of ComputeNode
type ComputeNodeStatus struct {
	State string `json:"state"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComputeNode is the Schema for the computenodes API
type ComputeNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComputeNodeSpec   `json:"spec,omitempty"`
	Status ComputeNodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComputeNodeList contains a list of ComputeNode
type ComputeNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComputeNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComputeNode{}, &ComputeNodeList{})
}
