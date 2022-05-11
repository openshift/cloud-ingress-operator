package ingresscontroller

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	corev1 "k8s.io/api/core/v1"
)

type IngressController struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IngressControllerSpec   `json:"spec,omitempty"`
	Status IngressControllerStatus `json:"status,omitempty"`
}

type IngressControllerSpec struct {
	DefaultCertificate         *corev1.LocalObjectReference `json:"defaultCertificate,omitempty"`
	NodePlacement              *NodePlacement               `json:"nodePlacement,omitempty"`
	Domain                     string                       `json:"domain,omitempty"`
	EndpointPublishingStrategy *EndpointPublishingStrategy  `json:"endpointPublishingStrategy,omitempty"`
	RouteSelector              *metav1.LabelSelector        `json:"routeSelector,omitempty"`
	Replicas                   *int32                       `json:"replicas,omitempty"`
}

type NodePlacement struct {
	NodeSelector *metav1.LabelSelector `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration   `json:"tolerations,omitempty"`
}

type EndpointPublishingStrategyType string

const (
	LoadBalancerServiceStrategyType EndpointPublishingStrategyType = "LoadBalancerService"
	HostNetworkStrategyType         EndpointPublishingStrategyType = "HostNetwork"
	PrivateStrategyType             EndpointPublishingStrategyType = "Private"
	NodePortServiceStrategyType     EndpointPublishingStrategyType = "NodePortService"
)

type LoadBalancerScope string

var (
	InternalLoadBalancer LoadBalancerScope = "Internal"
	ExternalLoadBalancer LoadBalancerScope = "External"
)

type LoadBalancerStrategy struct {
	Scope              LoadBalancerScope               `json:"scope"`
	ProviderParameters *ProviderLoadBalancerParameters `json:"providerParameters,omitempty"`
}

type ProviderLoadBalancerParameters struct {
	Type LoadBalancerProviderType   `json:"type"`
	AWS  *AWSLoadBalancerParameters `json:"aws,omitempty"`
	GCP  *GCPLoadBalancerParameters `json:"gcp,omitempty"`
}

type LoadBalancerProviderType string

const (
	AWSLoadBalancerProvider LoadBalancerProviderType = "AWS"
	GCPLoadBalancerProvider LoadBalancerProviderType = "GCP"
)

type AWSLoadBalancerParameters struct {
	Type                          AWSLoadBalancerType               `json:"type"`
	ClassicLoadBalancerParameters *AWSClassicLoadBalancerParameters `json:"classicLoadBalancer,omitempty"`
	NetworkLoadBalancerParameters *AWSNetworkLoadBalancerParameters `json:"networkLoadBalancer,omitempty"`
}

type AWSLoadBalancerType string

const (
	AWSClassicLoadBalancer AWSLoadBalancerType = "Classic"
	AWSNetworkLoadBalancer AWSLoadBalancerType = "NLB"
)

type GCPLoadBalancerParameters struct {
	ClientAccess GCPClientAccess `json:"clientAccess,omitempty"`
}

type GCPClientAccess string

const (
	GCPGlobalAccess GCPClientAccess = "Global"
	GCPLocalAccess  GCPClientAccess = "Local"
)

type AWSClassicLoadBalancerParameters struct {
}

type AWSNetworkLoadBalancerParameters struct {
}

type HostNetworkStrategy struct {
	Protocol IngressControllerProtocol `json:"protocol,omitempty"`
}

type PrivateStrategy struct {
}

type NodePortStrategy struct {
	Protocol IngressControllerProtocol `json:"protocol,omitempty"`
}

type IngressControllerProtocol string

const (
	DefaultProtocol IngressControllerProtocol = ""
	TCPProtocol     IngressControllerProtocol = "TCP"
	ProxyProtocol   IngressControllerProtocol = "PROXY"
)

type EndpointPublishingStrategy struct {
	Type         EndpointPublishingStrategyType `json:"type"`
	LoadBalancer *LoadBalancerStrategy          `json:"loadBalancer,omitempty"`
	HostNetwork  *HostNetworkStrategy           `json:"hostNetwork,omitempty"`
	Private      *PrivateStrategy               `json:"private,omitempty"`
	NodePort     *NodePortStrategy              `json:"nodePort,omitempty"`
}

var (
	IngressControllerAvailableConditionType = "Available"
	LoadBalancerManagedIngressConditionType = "LoadBalancerManaged"
	LoadBalancerReadyIngressConditionType   = "LoadBalancerReady"
	DNSManagedIngressConditionType          = "DNSManaged"
	DNSReadyIngressConditionType            = "DNSReady"
)

type IngressControllerStatus struct {
	AvailableReplicas          int32                       `json:"availableReplicas"`
	Selector                   string                      `json:"selector"`
	Domain                     string                      `json:"domain"`
	EndpointPublishingStrategy *EndpointPublishingStrategy `json:"endpointPublishingStrategy,omitempty"`
	Conditions                 []OperatorCondition         `json:"conditions,omitempty"`
	ObservedGeneration         int64                       `json:"observedGeneration,omitempty"`
}

type OperatorCondition struct {
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	LastTransitionTime metav1.Time     `json:"lastTransitionTime,omitempty"`
	Reason             string          `json:"reason,omitempty"`
	Message            string          `json:"message,omitempty"`
}

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

type IngressControllerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []IngressController `json:"items"`
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressController) DeepCopyInto(out *IngressController) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PublishingStrategy.
func (in *IngressController) DeepCopy() *IngressController {
	if in == nil {
		return nil
	}
	out := new(IngressController)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *IngressController) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressControllerList) DeepCopyInto(out *IngressControllerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]IngressController, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PublishingStrategyList.
func (in *IngressControllerList) DeepCopy() *IngressControllerList {
	if in == nil {
		return nil
	}
	out := new(IngressControllerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *IngressControllerList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressControllerSpec) DeepCopyInto(out *IngressControllerSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PublishingStrategySpec.
func (in *IngressControllerSpec) DeepCopy() *IngressControllerSpec {
	if in == nil {
		return nil
	}
	out := new(IngressControllerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *IngressControllerStatus) DeepCopyInto(out *IngressControllerStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PublishingStrategyStatus.
func (in *IngressControllerStatus) DeepCopy() *IngressControllerStatus {
	if in == nil {
		return nil
	}
	out := new(IngressControllerStatus)
	in.DeepCopyInto(out)
	return out
}
