package utils

import (
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpdateConditionCheck tests whether a condition should be updated from the
// old condition to the new condition. Returns true if the condition should
// be updated.
type UpdateConditionCheck func(oldReason, oldMessage, newReason, newMessage string) bool

// UpdateConditionAlways returns true. The condition will always be updated.
func UpdateConditionAlways(_, _, _, _ string) bool {
	return true
}

// UpdateConditionNever return false. The condition will never be updated,
// unless there is a change in the status of the condition.
func UpdateConditionNever(_, _, _, _ string) bool {
	return false
}

// UpdateConditionIfReasonOrMessageChange returns true if there is a change
// in the reason or the message of the condition.
func UpdateConditionIfReasonOrMessageChange(oldReason, oldMessage, newReason, newMessage string) bool {
	return oldReason != newReason ||
		oldMessage != newMessage
}

// SetAPISchemeCondition sets a condition on a APIScheme resource's status
func SetAPISchemeCondition(
	conditions []cloudingressv1alpha1.APISchemeCondition,
	conditionType cloudingressv1alpha1.APISchemeConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []cloudingressv1alpha1.APISchemeCondition {
	now := metav1.Now()
	existingCondition := FindAPISchemeCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				cloudingressv1alpha1.APISchemeCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
				},
			)
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
		}
	}
	return conditions
}

// FindAPISchemeCondition finds in the condition that has the matching condition type
func FindAPISchemeCondition(conditions []cloudingressv1alpha1.APISchemeCondition, conditionType cloudingressv1alpha1.APISchemeConditionType) *cloudingressv1alpha1.APISchemeCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func shouldUpdateCondition(
	oldStatus corev1.ConditionStatus, oldReason, oldMessage string,
	newStatus corev1.ConditionStatus, newReason, newMessage string,
	updateConditionCheck UpdateConditionCheck,
) bool {
	if oldStatus != newStatus {
		return true
	}
	return updateConditionCheck(oldReason, oldMessage, newReason, newMessage)
}
