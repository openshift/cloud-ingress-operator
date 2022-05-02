package validatingwebhooks

import (
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// TODO I have no idea what these values should actually be.
	sshdAdmissionGroup   = "cloudingress.managed.openshift.io"
	sshdAdmissionVersion = "v1alpha1"
)

// SSHDValidatingAdmissionHook adheres to interface ValidatingAdmissionHookV1Beta1 and is used by
// the generic-admission-server to determine which code to run to validate when changes are made to
// SSHD resources.
type SSHDValidatingAdmissionHook struct {
	decoder *admission.Decoder
}

func NewSSHDValidatingAdmissionHook(decoder *admission.Decoder) *SSHDValidatingAdmissionHook {
	return &SSHDValidatingAdmissionHook{
		decoder: decoder,
	}
}

func (a *SSHDValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	return nil
}

func (a *SSHDValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
		Group:    sshdAdmissionGroup,
		Version:  sshdAdmissionVersion,
		Resource: "sshds",
	}, "sshd"
}

func (a *SSHDValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	// Currently only the delete operation requires validation, if any operation other than delete
	// is performed, skip validation
	switch admissionSpec.Operation {
	case admissionv1beta1.Delete:
		return validateProtectedDelete(a.decoder, admissionSpec)
	default:
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
}
