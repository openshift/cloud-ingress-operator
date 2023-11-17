// This package is for low-level utility functions used by both controllers
// and CloudClient interface implementations.
package utils

import (
	"context"
	"encoding/json"
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	masterMachineLabel  string = "machine.openshift.io/cluster-api-machine-role"
	machineApiNamespace string = "openshift-machine-api"
	cpmsName            string = "cluster"
)

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

// GetControlPlaneMachineSet returns an OSD cluster's CPMS.
func GetControlPlaneMachineSet(kclient client.Client) (*machinev1.ControlPlaneMachineSet, error) {
	cpms := &machinev1.ControlPlaneMachineSet{}
	key := client.ObjectKey{
		Namespace: machineApiNamespace,
		Name:      cpmsName,
	}
	err := kclient.Get(context.TODO(), key, cpms)
	if err != nil {
		return nil, fmt.Errorf("failed to get controlplanemachineset: %w", err)
	}
	return cpms, nil
}

// DeleteCPMS will remove the CPMS of the cluster - in OSD this will trigger the
// CPMS to be recreated in an inactive state.
func DeleteCPMS(ctx context.Context, kclient client.Client, cpms *machinev1.ControlPlaneMachineSet) error {
	return kclient.Delete(ctx, cpms)
}

// SetCPMSActive will set a CPMS back to active.
// This is required after calling DeleteCPMS, as it will recreate the CPMS in an inactive state.
func SetCPMSActive(ctx context.Context, kclient client.Client, cpms *machinev1.ControlPlaneMachineSet) error {
	patch := client.MergeFrom(cpms.DeepCopy())
	cpms.Spec.State = machinev1.ControlPlaneMachineSetStateActive
	return kclient.Patch(ctx, cpms, patch)
}

func ConvertFromRawExtension[T any](extension *runtime.RawExtension) (*T, error) {
	t := new(T)
	if extension == nil {
		return t, fmt.Errorf("can not convert nil to type")
	}
	if err := json.Unmarshal(extension.Raw, &t); err != nil {
		return t, fmt.Errorf("error unmarshalling providerSpec: %v", err)
	}
	return t, nil
}

func ConvertToRawBytes(t interface{}) ([]byte, error) {
	raw, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("could not marshal provided type: %v", err)
	}
	return raw, nil
}
