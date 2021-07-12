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
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	sshdGroup    = "cloudingress.managed.openshift.io"
	sshdVersion  = "v1"
	sshdResource = "sshd"
)

type sshdDeleteAdmissionHook struct {
	decoder *admission.Decoder
}

// NewsshdDeleteAdmissionHook constructs a new SyncSetValidatingAdmissionHook
func NewsshdDeleteAdmissionHook(decoder *admission.Decoder) *sshdDeleteAdmissionHook {
	return &sshdDeleteAdmissionHook{decoder: decoder}
}

// validateDelete specifically validates delete operations for ClusterDeployment objects.
func (a *sshdDeleteAdmissionHook) validateDelete(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	logger := log.WithFields(log.Fields{
		"operation": request.Operation,
		"group":     request.Resource.Group,
		"version":   request.Resource.Version,
		"resource":  request.Resource.Resource,
		"method":    "validateDelete",
	})

	// If running on OpenShift 3.11, OldObject will not be populated. All we can do is accept the DELETE request.
	if len(request.OldObject.Raw) == 0 {
		logger.Info("Cannot validate the DELETE since OldObject is empty")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	oldObject := &cloudingressv1alpha1.SSHD{}
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
