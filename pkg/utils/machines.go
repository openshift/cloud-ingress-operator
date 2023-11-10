// This package is for low-level utility functions used by both controllers
// and CloudClient interface implementations.
package utils

import (
	"context"
	"encoding/json"
	"fmt"

	machinev1 "github.com/openshift/api/machine/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	masterMachineLabel  string = "machine.openshift.io/cluster-api-machine-role"
	machineApiNamespace string = "openshift-machine-api"
	cpmsName            string = "cluster"
)

var (
	log = logf.Log.WithName("baseutils")
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

// This function will:
// 1. Remove the CPMS
// 2. Modify the machines in the non-CPMS usage way.
// 3. Wait until N machines (= number of controlplane machines) have been completely deleted
// 4. Recreate the CPMS
func RemoveCPMSAndAwaitMachineRemoval(ctx context.Context, kclient client.Client, cpms *machinev1.ControlPlaneMachineSet) error {
	log.Info("Removing INACTIVE CPMS to allow machine modifications")
	err := kclient.Delete(ctx, cpms)
	if err != nil {
		log.Error(err, "Removing CPMS failed")
		return err
	}
	// This must run async, if this blocks the cluster will break itself,
	// because the API connectivity will break without the changes happening
	// later (e.g. updating Route53)
	go func() {
		// Inside the goroutine retrieve the list of masters instead of closing
		// over it. Pointers could easily point to wrong data otherwise.
		masterList, err := GetMasterMachines(kclient)
		if err != nil {
			log.Error(err, "Could not get master machines")
			return
		}
		log.Info("Waiting for master machines to be removed and recreate CPMS at the end.")
		scheme := runtime.NewScheme()
		err = machinev1.AddToScheme(scheme)
		if err != nil {
			log.Error(err, "Could not add machinev1 api to scheme")
			return
		}
		err = machineapi.AddToScheme(scheme)
		if err != nil {
			log.Error(err, "Could not add machinev1beta1 api to scheme")
			return
		}
		watchClient, err := client.NewWithWatch(config.GetConfigOrDie(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			log.Error(err, "Could not create watcher client")
			return
		}
		watcher, err := watchClient.Watch(ctx, masterList)
		if err != nil {
			log.Error(err, "Could not create watch")
			return
		}
		removedMachines := 0
		totalMachines := len(masterList.Items)
		for event := range watcher.ResultChan() {
			switch event.Type {
			case watch.Deleted:
				removedMachines += 1
				if removedMachines == totalMachines {
					// TODO: With Luck this is good enough, as all new machines should be 'good' for the CPMS?
					log.Info("All master machines have been removed now.")
					watcher.Stop()
				}
			}
		}
		// Remove fields that might prevent recreation
		cpms.Status = machinev1.ControlPlaneMachineSetStatus{}
		cpms.CreationTimestamp = v1.Time{}
		cpms.ResourceVersion = ""
		// At least always attempt to recreate the cpms
		err = kclient.Create(ctx, cpms)
		if err != nil {
			log.Error(err, "Could not recreate CPMS")
		}
	}()
	return nil
}
