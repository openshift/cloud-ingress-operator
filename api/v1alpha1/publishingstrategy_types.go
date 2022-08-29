/*
Copyright 2022.

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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PublishingStrategySpec defines the desired state of PublishingStrategy
type PublishingStrategySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file

	// DefaultAPIServerIngress defines whether API is internal or external
	DefaultAPIServerIngress DefaultAPIServerIngress `json:"defaultAPIServerIngress"`
	//ApplicationIngress defines whether application ingress is internal or external
	ApplicationIngress []ApplicationIngress `json:"applicationIngress"`
}

// DefaultAPIServerIngress defines API ingress
type DefaultAPIServerIngress struct {
	// Listening defines internal or external ingress
	Listening Listening `json:"listening,omitempty"`
}

// ApplicationIngress defines application ingress
type ApplicationIngress struct {
	// Listening defines application ingress as internal or external
	Listening Listening `json:"listening,omitempty"`
	// Default defines default value of ingress when cluster installs
	Default       bool                   `json:"default"`
	DNSName       string                 `json:"dnsName"`
	Certificate   corev1.SecretReference `json:"certificate"`
	RouteSelector metav1.LabelSelector   `json:"routeSelector,omitempty"`
	Type          Type                   `json:"type,omitempty"`
}

// Listening defines internal or external api and ingress
type Listening string

// Type indicates the type of Load Balancer to use
// +kubebuilder:validation:Enum=Classic;NLB
type Type string

const (
	// Internal const for listening status
	Internal Listening = "internal"
	// External const for listening status
	External Listening = "external"
)

// PublishingStrategyStatus defines the observed state of PublishingStrategy
type PublishingStrategyStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PublishingStrategy is the Schema for the publishingstrategies API
type PublishingStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PublishingStrategySpec   `json:"spec"`
	Status PublishingStrategyStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PublishingStrategyList contains a list of PublishingStrategy
type PublishingStrategyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PublishingStrategy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PublishingStrategy{}, &PublishingStrategyList{})
}
