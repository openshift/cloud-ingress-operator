package utils

import (
	"context"
	"reflect"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logf.Log.WithName("machine_helper")

// RemoveAWSLBFromMasterMachines removes a Load Balancer (with name elbName) from
// the spec.providerSpec.value.loadBalancers list for each of the master machine
// objects in a cluster
func RemoveAWSLBFromMasterMachines(kclient client.Client, elbName string, masterNodes *machineapi.MachineList) error {
	for _, machine := range masterNodes.Items {
		providerSpecDecoded, err := getAWSDecodedProviderSpec(machine)
		if err != nil {
			log.Error(err, "Error retrieving decoded ProviderSpec for machine", "machine", machine.Name)
			return err
		}
		lbList := providerSpecDecoded.LoadBalancers
		newLBList := []awsproviderapi.LoadBalancerReference{}
		for _, lb := range lbList {
			if lb.Name != elbName {
				log.Info("Machine's LB does not match LB to remove", "Machine LB", lb.Name, "LB to remove", elbName)
				log.Info("Keeping machine's LB in machine object", "LB", lb.Name, "Machine", machine.Name)
				newLBList = append(newLBList, lb)
			}
		}
		err = updateAWSLBList(kclient, lbList, newLBList, machine, providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error updating LB list for machine", "machine", machine.Name)
			return err
		}
	}
	return nil
}

// AddAWSLBToMasterMachines adds a Load Balancer (with name elbName) to the
// spec.providerSpec.value.loadBalancers list for each of the master machine
// objects in a cluster
func AddAWSLBToMasterMachines(kclient client.Client, elbName string, masterNodes *machineapi.MachineList) error {
	for _, machine := range masterNodes.Items {
		providerSpecDecoded, err := getAWSDecodedProviderSpec(machine)
		if err != nil {
			log.Error(err, "Error retrieving decoded ProviderSpec for machine", "machine", machine.Name)
			return err
		}
		lbList := providerSpecDecoded.LoadBalancers
		newLBList := []awsproviderapi.LoadBalancerReference{}
		for _, lb := range lbList {
			if lb.Name != elbName {
				newLBList = append(newLBList, lb)
			}
		}
		newLB := awsproviderapi.LoadBalancerReference{
			Name: elbName,
			Type: awsproviderapi.NetworkLoadBalancerType,
		}
		newLBList = append(newLBList, newLB)
		err = updateAWSLBList(kclient, lbList, newLBList, machine, providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error updating LB list for machine", "machine", machine.Name)
			return err
		}
	}
	return nil
}

// getAWSDecodedProviderSpec casts the spec.providerSpec of a kubernetes machine
// object to an AWSMachineProviderConfig object, which is required to read and
// interact with fields in a machine's providerSpec
func getAWSDecodedProviderSpec(machine machineapi.Machine) (*awsproviderapi.AWSMachineProviderConfig, error) {
	awsCodec, err := awsproviderapi.NewCodec()
	if err != nil {
		log.Error(err, "Error creating AWSProviderConfigCodec")
		return nil, err
	}
	providerSpecEncoded := machine.Spec.ProviderSpec
	providerSpecDecoded := &awsproviderapi.AWSMachineProviderConfig{}
	err = awsCodec.DecodeProviderSpec(&providerSpecEncoded, providerSpecDecoded)
	if err != nil {
		log.Error(err, "Error decoding provider spec for machine", "machine", machine.Name)
		return nil, err
	}
	return providerSpecDecoded, nil
}

// updateAWSLBList compares an oldLBList to a newLBList and updates a machine
// object's spec.providerSpec.value.loadBalancers list with the newLBList if
// the old and new lists are not equal. this function requires the decoded
// ProviderSpec (as an AWSMachineProviderConfig object) that the
// getAWSDecodedProviderSpec function will provide
func updateAWSLBList(kclient client.Client, oldLBList []awsproviderapi.LoadBalancerReference, newLBList []awsproviderapi.LoadBalancerReference, machineToPatch machineapi.Machine, providerSpecDecoded *awsproviderapi.AWSMachineProviderConfig) error {
	baseToPatch := client.MergeFrom(machineToPatch.DeepCopy())
	awsCodec, err := awsproviderapi.NewCodec()
	if err != nil {
		log.Error(err, "Error creating AWSProviderConfigCodec")
		return err
	}
	if !reflect.DeepEqual(oldLBList, newLBList) {
		providerSpecDecoded.LoadBalancers = newLBList
		newProviderSpecEncoded, err := awsCodec.EncodeProviderSpec(providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error encoding provider spec for machine", "machine", machineToPatch.Name)
			return err
		}
		machineToPatch.Spec.ProviderSpec = *newProviderSpecEncoded
		machineObj := machineToPatch.DeepCopyObject()
		if err := kclient.Patch(context.Background(), machineObj, baseToPatch); err != nil {
			log.Error(err, "Failed to update LBs in machine's providerSpec", "machine", machineToPatch.Name)
			return err
		}
		log.Info("Updated master machine's LBs in providerSpec", "masterMachine", machineToPatch.Name)
		return nil
	}
	log.Info("No need to update LBs for master machine", "masterMachine", machineToPatch.Name)
	return nil
}
