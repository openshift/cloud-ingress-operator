package publishingstrategy

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	operatorv1 "github.com/openshift/api/operator/v1"
	ctlutils "github.com/openshift/cloud-ingress-operator/pkg/controller/utils"
)

// addFinalizer adds Finalizer to an IngressController
func (r *ReconcilePublishingStrategy) addFinalizer(reqLogger logr.Logger, ingressController *operatorv1.IngressController, finalizer string) error {
	reqLogger.Info(fmt.Sprintf("Adding Finalizer %v for the IngressController %v", finalizer, ingressController.Name))
	ingressController.SetFinalizers(append(ingressController.GetFinalizers(), finalizer))

	// Update CR
	err := r.client.Update(context.TODO(), ingressController)
	if err != nil {
		reqLogger.Error(err, "Failed to update IngressController with finalizer")
		return err
	}
	return nil
}

// removeFinalizer removes a Finalizer from an IngressController
func (r *ReconcilePublishingStrategy) removeFinalizer(reqLogger logr.Logger, ingressController *operatorv1.IngressController, finalizer string) error {
	reqLogger.Info(fmt.Sprintf("Removing Finalizer %v for the IngressController %v", finalizer, ingressController.Name))
	ingressController.SetFinalizers(ctlutils.Remove(ingressController.GetFinalizers(), finalizer))

	// Update CR
	err := r.client.Update(context.TODO(), ingressController)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to remove Finalizer %v", finalizer))
		return err
	}
	return nil
}
