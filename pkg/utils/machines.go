// This package is for low-level utility functions used by both controllers
// and CloudClient interface implementations.
package utils

import (
	"context"

	machineapi "github.com/openshift/api/machine/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const masterMachineLabel string = "machine.openshift.io/cluster-api-machine-role"

// GetMasterMachines returns a MachineList object whose .Items can be iterated
// over to perform actions on/with information from each master machine object.
func GetMasterMachines(kclient client.Client) (*machineapi.MachineList, error) {
	machineList := &machineapi.MachineList{}
	listOptions := []client.ListOption{
		client.InNamespace("openshift-machine-api"),
		client.MatchingLabels{masterMachineLabel: "master"},
	}
	err := kclient.List(context.TODO(), machineList, listOptions...)
	if err != nil {
		return nil, err
	}
	return machineList, nil
}
