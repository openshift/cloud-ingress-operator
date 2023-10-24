// This package is for low-level utility functions used by both controllers
// and CloudClient interface implementations.
package utils

import (
	"context"
	"fmt"
	machinev1 "github.com/openshift/api/machine/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	machineapi "github.com/openshift/api/machine/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	masterMachineLabel  = "machine.openshift.io/cluster-api-machine-role"
	machineApiNamespace = "openshift-machine-api"
	cpmsName            = "cluster"
)

// GetMasterMachines returns a MachineList object whose .Items can be iterated
// over to perform actions on/with information from each master machine object.
func GetMasterMachines(kclient client.Client) (*machineapi.MachineList, error) {
	machineList := &machineapi.MachineList{}
	listOptions := []client.ListOption{
		client.InNamespace(machineApiNamespace),
		client.MatchingLabels{masterMachineLabel: "master"},
	}
	err := kclient.List(context.TODO(), machineList, listOptions...)
	if err != nil {
		return nil, err
	}
	return machineList, nil
}

func GetControlPlaneMachineSet(kclient client.Client) (*machinev1.ControlPlaneMachineSet, error) {
	cpms := &machinev1.ControlPlaneMachineSet{}
	key := client.ObjectKey{
		Namespace: machineApiNamespace,
		Name:      cpmsName,
	}
	err := kclient.Get(context.TODO(), key, cpms)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Nothing to do
		}
		return nil, fmt.Errorf("failed to get controlplanemachineset: %w", err)
	}
	return cpms, nil
}
