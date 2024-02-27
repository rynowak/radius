/*
Copyright 2023.

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

package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ApplicationDeploymentSpec defines the desired state of an ApplicationDeployment
type ApplicationDeploymentSpec struct {
	// Environment is the name of the Radius environment to use. If unset the value 'default' will be
	// used as the environment name.
	Environment string `json:"environment,omitempty"`

	// Application is the name of the Radius application to use. If unset the namespace of the
	// ApplicationDeployment will be used as the application name.
	Application string `json:"application,omitempty"`

	// Template is the compiled ARM-JSON template that will be reconciled.
	Template string `json:"template,omitempty"`
}

// ApplicationDeploymentPhrase is a string representation of the current status of an Application Deployment.
type ApplicationDeploymentPhrase string

const (
	// ApplicationDeploymentPhraseUpdating indicates that the Application Deployment is being updated.
	ApplicationDeploymentPhraseUpdating ApplicationDeploymentPhrase = "Updating"

	// ApplicationDeploymentPhraseReady indicates that the Application Deployment is ready.
	ApplicationDeploymentPhraseReady ApplicationDeploymentPhrase = "Ready"

	// ApplicationDeploymentPhraseFailed indicates that the Application Deployment has failed.
	ApplicationDeploymentPhraseFailed ApplicationDeploymentPhrase = "Failed"

	// ApplicationDeploymentPhraseDeleting indicates that the Application Deployment is being deleted.
	ApplicationDeploymentPhraseDeleting ApplicationDeploymentPhrase = "Deleting"

	// ApplicationDeploymentPhraseDeleted indicates that the Application Deployment has been deleted.
	ApplicationDeploymentPhraseDeleted ApplicationDeploymentPhrase = "Deleted"
)

// ApplicationDeploymentStatus defines the observed state of the Application Deployment.
type ApplicationDeploymentStatus struct {
	// ObservedGeneration is the most recent generation observed for this Application Deployment. It corresponds to the
	// Deployment's generation, which is updated on mutation by the API Server.
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,1,opt,name=observedGeneration"`

	// Application is the resource ID of the application.
	// +kubebuilder:validation:Optional
	Application string `json:"application,omitempty"`

	// Environment is the resource ID of the environment.
	// +kubebuilder:validation:Optional
	Environment string `json:"environment,omitempty"`

	// Scope is the resource ID of the scope.
	// +kubebuilder:validation:Optional
	Scope string `json:"scope,omitempty"`

	// Resource is the resource ID of the deployment.
	// +kubebuilder:validation:Optional
	Resource string `json:"resource,omitempty"`

	// Operation tracks the status of an in-progress provisioning operation.
	// +kubebuilder:validation:Optional
	Operation *ResourceOperation `json:"operation,omitempty"`

	// Phrase indicates the current status of the Application Deployment.
	// +kubebuilder:validation:Optional
	Phrase ApplicationDeploymentPhrase `json:"phrase,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:categories={"all","radius"}
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phrase",description="Status of the resource"
//+kubebuilder:subresource:status

// ApplicationDeployment is the Schema for the Application Deployment API.
type ApplicationDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationDeploymentSpec   `json:"spec,omitempty"`
	Status ApplicationDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ApplicationDeploymentList contains a list of ApplicationDeployment objects.
type ApplicationDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApplicationDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApplicationDeployment{}, &ApplicationDeploymentList{})
}
