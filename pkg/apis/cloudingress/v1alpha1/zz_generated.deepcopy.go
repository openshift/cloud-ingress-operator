// +build !ignore_autogenerated

// Code generated by operator-sdk. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *APIScheme) DeepCopyInto(out *APIScheme) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new APIScheme.
func (in *APIScheme) DeepCopy() *APIScheme {
	if in == nil {
		return nil
	}
	out := new(APIScheme)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *APIScheme) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *APISchemeList) DeepCopyInto(out *APISchemeList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]APIScheme, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new APISchemeList.
func (in *APISchemeList) DeepCopy() *APISchemeList {
	if in == nil {
		return nil
	}
	out := new(APISchemeList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *APISchemeList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *APISchemeSpec) DeepCopyInto(out *APISchemeSpec) {
	*out = *in
	in.ManagementAPIServerIngress.DeepCopyInto(&out.ManagementAPIServerIngress)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new APISchemeSpec.
func (in *APISchemeSpec) DeepCopy() *APISchemeSpec {
	if in == nil {
		return nil
	}
	out := new(APISchemeSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *APISchemeStatus) DeepCopyInto(out *APISchemeStatus) {
	*out = *in
	if in.HistoricConditions != nil {
		in, out := &in.HistoricConditions, &out.HistoricConditions
		*out = make([]HistoricCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new APISchemeStatus.
func (in *APISchemeStatus) DeepCopy() *APISchemeStatus {
	if in == nil {
		return nil
	}
	out := new(APISchemeStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HistoricCondition) DeepCopyInto(out *HistoricCondition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
	if in.AllowedCIDRBlocks != nil {
		in, out := &in.AllowedCIDRBlocks, &out.AllowedCIDRBlocks
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HistoricCondition.
func (in *HistoricCondition) DeepCopy() *HistoricCondition {
	if in == nil {
		return nil
	}
	out := new(HistoricCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ManagementAPIServerIngress) DeepCopyInto(out *ManagementAPIServerIngress) {
	*out = *in
	if in.AllowedCIDRBlocks != nil {
		in, out := &in.AllowedCIDRBlocks, &out.AllowedCIDRBlocks
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ManagementAPIServerIngress.
func (in *ManagementAPIServerIngress) DeepCopy() *ManagementAPIServerIngress {
	if in == nil {
		return nil
	}
	out := new(ManagementAPIServerIngress)
	in.DeepCopyInto(out)
	return out
}
