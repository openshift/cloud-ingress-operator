package publishingstrategy

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctlutils "github.com/openshift/cloud-ingress-operator/pkg/controllerutils"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
)

// addFinalizer adds Finalizer to an IngressController
func (r *PublishingStrategyReconciler) addFinalizer(reqLogger logr.Logger, ingressController *ingresscontroller.IngressController, finalizer string) error {
	reqLogger.Info(fmt.Sprintf("Adding Finalizer %v for the IngressController %v", finalizer, ingressController.Name))
	ingressController.SetFinalizers(append(ingressController.GetFinalizers(), finalizer))

	// Update CR
	err := r.Client.Update(context.TODO(), ingressController)
	if err != nil {
		reqLogger.Error(err, "Failed to update IngressController with finalizer")
		return err
	}
	return nil
}

// removeFinalizer removes a Finalizer from an IngressController
func (r *PublishingStrategyReconciler) removeFinalizer(reqLogger logr.Logger, ingressController *ingresscontroller.IngressController, finalizer string) error {
	reqLogger.Info(fmt.Sprintf("Removing Finalizer %v for the IngressController %v", finalizer, ingressController.Name))
	ingressController.SetFinalizers(ctlutils.Remove(ingressController.GetFinalizers(), finalizer))

	// Update CR
	err := r.Client.Update(context.TODO(), ingressController)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("Failed to remove Finalizer %v", finalizer))
		return err
	}
	return nil
}
