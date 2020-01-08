package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ManagementState is to represent where we are in the state machine
// Pending -> CreatingLoadBalancer -> UpdatingLoadBalancerListeners ->
// UpdatingCIDRAllowances -> UpdatingDNS -> UpdatingAPIServer -> Ready
// With errors possible
type ManagementState uint

// TODO: helper function to turn these into strings
const (
	Pending ManagementState = iota
	Ready
	Error
	CreatingLoadBalancer
	UpdatingLoadBalancerListeners
	UpdatingCIDRAllowances
	UpdatingDNS
	UpdatingAPIServer
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ApiSchemeSpec defines the desired state of ApiScheme
// +k8s:openapi-gen=true
type ApiSchemeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	ManagementAPIServerIngress ManagementAPIServerIngress `json:",managementAPIServerIngress"`
}

// ManagementAPIServerIngress defines the Management API ingress
type ManagementAPIServerIngress struct {
	Enabled           bool     `json:",enabled"`
	DNSName           string   `json:",dnsName"`
	AllowedCIDRBlocks []string `json:",allowedCIDRBlocks"`
}

// ApiSchemeStatus defines the observed state of ApiScheme
// +k8s:openapi-gen=true
type ApiSchemeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	HistoricConditions []HistoricCondition `json:",history"`

	// State is the state machine
	State ManagementState `json:",state"`
}

// HistoricCondition is the history of transitions
type HistoricCondition struct {
	// LastTransitionTime Last change to status
	LastTransitionTime metav1.Time `json:",lastTransitionTime"`

	// AllowedCIDRBlocks currently allowed (as of the last successful Security Group update)
	AllowedCIDRBlocks []string `json:",allowedCIDRBlocks,omitempty"`

	// Reason is why we're making this status change
	Reason string `json:",reason"`

	// Message is an English text
	Message string `json:",message"`

	// Status is the string representation of the state machine (See ManagementState)
	Status corev1.ConditionStatus `json:",status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApiScheme is the Schema for the apischemes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=apischemes,scope=Namespaced
type ApiScheme struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApiSchemeSpec   `json:"spec,omitempty"`
	Status ApiSchemeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ApiSchemeList contains a list of ApiScheme
type ApiSchemeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApiScheme `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ApiScheme{}, &ApiSchemeList{})
}
