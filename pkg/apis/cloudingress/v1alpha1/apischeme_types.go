package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APISchemeConditionType is to represent where we are in the state machine
// Pending -> CreatingLoadBalancer -> UpdatingLoadBalancerListeners ->
// UpdatingCIDRAllowances -> UpdatingDNS -> UpdatingAPIServer -> Ready
// With errors possible
type APISchemeConditionType string

const (
	// APISchemePending is set for pending CR
	APISchemePending APISchemeConditionType = "Pending"
	// APISchemeError is set when we have an unrecoverable error
	APISchemeError APISchemeConditionType = "Error"
	// APISchemeCreatedLoadBalancer is set after we create the LB
	APISchemeCreatedLoadBalancer APISchemeConditionType = "CreatedLoadBalancer"
	// APISchemeUpdatedLoadBalancerListeners is set after we update the api LB listeners
	APISchemeUpdatedLoadBalancerListeners APISchemeConditionType = "UpdatedLoadBalancerListeners"
	// APISchemeUpdatedCIDRAllowances is set after we update CIDRs in LB's SG
	APISchemeUpdatedCIDRAllowances APISchemeConditionType = "UpdatedCIDRAllowances"
	// APISchemeUpdatedDNS is set after we update DNS settings
	APISchemeUpdatedDNS APISchemeConditionType = "UpdatedDNS"
	// APISchemeReady is set when all the steps are completed
	APISchemeReady APISchemeConditionType = "Ready"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// APISchemeSpec defines the desired state of APIScheme
// +k8s:openapi-gen=true
type APISchemeSpec struct {
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

// APISchemeStatus defines the observed state of APIScheme
// +k8s:openapi-gen=true
type APISchemeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Conditions       []APISchemeCondition `json:",conditions"`
	CloudLoadBalancerDNSName string              `json:",cloudLoadBalancerDNSName,omitempty"`

	// State is the state machine
	State ManagementState `json:",state"`
}

// APISchemeCondition is the history of transitions
type APISchemeCondition struct {
	// Type is the type of condition
	Type APISchemeConditionType `json:"type,omitempty"`

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

// APIScheme is the Schema for the APISchemes API
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=APISchemes,scope=Namespaced
type APIScheme struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   APISchemeSpec   `json:"spec,omitempty"`
	Status APISchemeStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// APISchemeList contains a list of APIScheme
type APISchemeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []APIScheme `json:"items"`
}

func init() {
	SchemeBuilder.Register(&APIScheme{}, &APISchemeList{})
}
