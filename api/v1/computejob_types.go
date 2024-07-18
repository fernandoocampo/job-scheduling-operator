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

// NodeSelector criteria for selecting nodes to run the job
type NodeSelector struct {
	// CPU required by the job
	// +kubebuilder:validation:Minimum=1
	CPU *int32 `json:"cpu,omitempty"`
	// Node name to run the job
	NodeName *string `json:"nodeName,omitempty"`
}

// ComputeJobSpec defines the desired state of ComputeJob
type ComputeJobSpec struct {
	// The command to run as a job
	Command []string `json:"command"`
	// Criteria for selecting nodes to run the job
	NodeSelector *NodeSelector `json:"nodeSelector,omitempty"`
	// The number of nodes the job should run on simultaneously
	// +kubebuilder:validation:Minimum=1
	Parallelism int32 `json:"parallelism"`
}

// ComputeJobStatus defines the observed state of ComputeJob
type ComputeJobStatus struct {
	State       string `json:"state"`
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
	ActiveNodes string `json:"activeNodes"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ComputeJob is the Schema for the computejobs API
type ComputeJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComputeJobSpec   `json:"spec,omitempty"`
	Status ComputeJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ComputeJobList contains a list of ComputeJob
type ComputeJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComputeJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComputeJob{}, &ComputeJobList{})
}
