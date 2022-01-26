package validatingwebhooks

import (
	"net/http"
	"strconv"

	"github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// TODO REMOVE ME
// Most of this is based on the examples in github.com/openshift/hive/pkg/validating-webhooks

const (
	// TODO I have no idea what these values should actually be.
	apiSchemeAdmissionGroup   = "admission.managed.openshift.io"
	apiSchemeAdmissionVersion = "v1"

	protectedDeleteAnnotation = "managed.openshift.io/protected-delete"
)

// APISchemeValidatingAdmissionHook adheres to interface ValidatingAdmissionHookV1Beta1 and is used by
// the generic-admission-server to determine which code to run to validate when changes are made to
// APIScheme resources.
type APISchemeValidatingAdmissionHook struct {
	decoder *admission.Decoder
}

// TODO make this work, not sure which package this is from?
//var _ admission.ValidatingAdmissionHookV1Beta1 = APISchemeValidatingAdmissionHook{}

func NewAPISchemeValidatingAdmissionHook(decoder *admission.Decoder) *APISchemeValidatingAdmissionHook {
	return &APISchemeValidatingAdmissionHook{
		decoder: decoder,
	}
}

func (a *APISchemeValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	return nil
}

func (a *APISchemeValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
		Group:    apiSchemeAdmissionGroup,
		Version:  apiSchemeAdmissionVersion,
		Resource: "apischemevalidators",
	}, "apischemevalidator"
}

func (a *APISchemeValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
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

// This should remain fairly generic to be able to be used by multiple resource validating webhooks
func validateProtectedDelete(decoder *admission.Decoder, admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	// If running on OpenShift 3.11, OldObject will not be populated. All we can do is accept the DELETE request.
	if len(admissionSpec.OldObject.Raw) == 0 {
		// TODO Add logging like the below using whatever CIO usually uses
		// logger.Info("Cannot validate the DELETE since OldObject is empty")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	// Decode the old object provided in the request spec
	oldObject := &v1alpha1.APIScheme{}
	if err := decoder.DecodeRaw(admissionSpec.OldObject, oldObject); err != nil {
		// If we're unable to marshal the object, something is very wrong, return not allowed
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	errs := field.ErrorList{}
	// Check for the protected delete annotation on the APIScheme resource that prevents deletion
	if value, present := oldObject.Annotations[protectedDeleteAnnotation]; present {
		if enabled, err := strconv.ParseBool(value); enabled && err == nil {
			errs = append(errs, field.Invalid(
				field.NewPath("metadata", "annotations", protectedDeleteAnnotation),
				oldObject.Annotations[protectedDeleteAnnotation],
				"cannot delete while annotation is present",
			))
			sch := schema.GroupKind{
				Group: admissionSpec.Kind.Group,
				Kind:  admissionSpec.Kind.Kind,
			}
			status := errors.NewInvalid(sch, admissionSpec.Name, errs).Status()
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result:  &status,
			}
		}
	}

	// If no validations returned, allow this operation
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}
