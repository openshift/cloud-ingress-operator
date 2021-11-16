package cio

import (
	"net/http"
	"strconv"

	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	"github.com/openshift/cloud-ingress-operator/pkg/constants"
	log "github.com/sirupsen/logrus"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//const (
//	apischemeGroup    = "cloudingress.managed.openshift.io"
//	apischemeVersion  = "v1"
//	apischemeResource = "apischeme"
//)

type ApischemeDeleteAdmissionHook struct {
	decoder *admission.Decoder
}

// NewApischemeDeleteAdmissionHook constructs a new SyncSetValidatingAdmissionHook
func NewApischemeDeleteAdmissionHook(decoder *admission.Decoder) *ApischemeDeleteAdmissionHook {
	return &ApischemeDeleteAdmissionHook{decoder: decoder}
}

// validateDelete specifically validates delete operations for ClusterDeployment objects.
func (a *ApischemeDeleteAdmissionHook) validateDelete(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	//reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	logger := log.WithFields(log.Fields{
		"operation": request.Operation,
		"group":     request.Resource.Group,
		"version":   request.Resource.Version,
		"resource":  request.Resource.Resource,
		"method":    "validateDelete",
	})
	logger.Info("Apischeme webhook at work")
	// If running on OpenShift 3.11, OldObject will not be populated. All we can do is accept the DELETE request.
	if len(request.OldObject.Raw) == 0 {
		logger.Info("Cannot validate the DELETE since OldObject is empty")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	oldObject := &cloudingressv1alpha1.APIScheme{}
	if err := a.decoder.DecodeRaw(request.OldObject, oldObject); err != nil {
		logger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	logger.Data["object.Name"] = oldObject.Name

	var allErrs field.ErrorList

	if value, present := oldObject.Annotations[constants.ProtectedDeleteAnnotation]; present {
		if enabled, err := strconv.ParseBool(value); enabled && err == nil {
			allErrs = append(allErrs, field.Invalid(
				field.NewPath("metadata", "annotations", constants.ProtectedDeleteAnnotation),
				oldObject.Annotations[constants.ProtectedDeleteAnnotation],
				"cannot delete while annotation is present",
			))
		} else {
			logger.WithField(constants.ProtectedDeleteAnnotation, value).Info("Protected Delete annotation present but not set to true")
		}
	}

	if len(allErrs) > 0 {
		logger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(request.Kind).GroupKind(), request.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	logger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}
func schemaGVK(gvk metav1.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}
func (a *ApischemeDeleteAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "Validate",
	})

	//	if !a.shouldValidate(admissionSpec) {
	//		contextLogger.Info("Skipping validation for request")
	//		// The request object isn't something that this validator should validate.
	//		// Therefore, we say that it's Allowed.
	//		return &admissionv1beta1.AdmissionResponse{
	//			Allowed: true,
	//		}
	//	}

	contextLogger.Info("Validating request")

	if admissionv1beta1.Delete == "DELETE" {
		return a.validateDelete(admissionSpec)
	} else {
		contextLogger.Info("Successful validation")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
}
