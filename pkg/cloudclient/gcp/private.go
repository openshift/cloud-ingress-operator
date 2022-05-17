package gcp

// "Private" or non-interface conforming methods

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"reflect"

	"google.golang.org/api/compute/v1"
	gdnsv1 "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"

	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	gcpproviderapi "github.com/openshift/cluster-api-provider-gcp/pkg/apis/gcpprovider/v1beta1"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	k8s "sigs.k8s.io/controller-runtime/pkg/client"

	cioerrors "github.com/openshift/cloud-ingress-operator/pkg/errors"
)

// ensureAdminAPIDNS ensures the DNS record for the "admin API" Service
// LoadBalancer is accurately set
func (gc *Client) ensureAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return gc.ensureDNSForService(kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName)
}

// deleteAdminAPIDNS ensures the DNS record for the "admin API" Service
// LoadBalancer is deleted
func (gc *Client) deleteAdminAPIDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return gc.removeDNSForService(kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName)
}

// ensureSSHDNS ensures the DNS record for the SSH Service LoadBalancer
// is accurately set
func (gc *Client) ensureSSHDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return gc.ensureDNSForService(kclient, svc, instance.Spec.DNSName)
}

// deleteSSHDNS ensures the DNS record for the SSH Service LoadBalancer
// is deleted
func (gc *Client) deleteSSHDNS(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return gc.removeDNSForService(kclient, svc, instance.Spec.DNSName)
}

// setDefaultAPIPrivate sets the default api (api.<cluster-domain>) to private
// scope
func (gc *Client) setDefaultAPIPrivate(ctx context.Context, kclient k8s.Client, _ *cloudingressv1alpha1.PublishingStrategy) error {
	intIPAddress, err := gc.removeLoadBalancerFromMasterNodes(ctx, kclient)
	if err != nil {
		return fmt.Errorf("Failed to remove load balancer from master nodes: %v", err)
	}
	apiDNSName := fmt.Sprintf("api.%s.", gc.baseDomain)
	oldIP, err := gc.updateAPIARecord(kclient, apiDNSName, intIPAddress)
	if err != nil {
		return err
	}
	// If the IP wasn't updated, there is nothing else to do
	if oldIP == intIPAddress {
		return nil
	}
	staticIPName := gc.clusterName + "-cluster-public-ip"
	err = gc.releaseExternalIP(staticIPName)
	if err != nil {
		return err
	}
	log.Info("Succcessfully set default API to private", "URL", apiDNSName, "IP Address", intIPAddress)
	return nil
}

// setDefaultAPIPublic sets the default API (api.<cluster-domain>) to public
// scope
func (gc *Client) setDefaultAPIPublic(ctx context.Context, kclient k8s.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	listCall := gc.computeService.ForwardingRules.List(gc.projectID, gc.region)
	response, err := listCall.Do()
	if err != nil {
		return err
	}
	// Create a new external LB
	//GCP ForwardingRule and TargetPool share the same name
	extNLBName := gc.clusterName + "-api"
	staticIPName := gc.clusterName + "-cluster-public-ip"
	for _, lb := range response.Items {
		// This list of forwardingrules (LBs) includes any service LBs
		// for application routers so check the port range to identify
		// the external API LB.
		if lb.LoadBalancingScheme == "EXTERNAL" && lb.PortRange == "6443-6443" && lb.Name == extNLBName {
			// If there is already an external LB serving over the API port, there is nothing to do.
			return nil
		}
	}
	staticIPAddress, err := gc.createExternalIP(staticIPName, "EXTERNAL")
	if err != nil {
		return err
	}
	err = gc.createNetworkLoadBalancer(extNLBName, "EXTERNAL", extNLBName, staticIPAddress)
	if err != nil {
		return err
	}
	apiDNSName := fmt.Sprintf("api.%s.", gc.baseDomain)
	_, err = gc.updateAPIARecord(kclient, apiDNSName, staticIPAddress)
	if err != nil {
		return err
	}
	log.Info("Successfully set default API load balancer to external", "URL", apiDNSName, "IP address", staticIPAddress)
	return nil
}

func (gc *Client) ensureDNSForService(kclient k8s.Client, svc *corev1.Service, dnsName string) error {
	// google.golang.org/api/dns/v1.Service is a struct, not an interface, which
	// will make this all but impossible to write unit tests for

	// Forwarding rule is necessary for rh-api lb setup
	// Check forwarding rule exists first
	ingressList := svc.Status.LoadBalancer.Ingress
	if len(ingressList) == 0 {
		// the LB doesn't exist
		return cioerrors.NewLoadBalancerNotReadyError()
	}
	rhapiLbIP := ingressList[0].IP
	// ensure forwarding rule exists in GCP for service
	err := gc.ensureGCPForwardingRuleForExtIP(rhapiLbIP)
	if err != nil {
		return cioerrors.ForwardingRuleNotFound(err.Error())
	}

	svcIPs, err := getIPAddressesFromService(svc)
	if err != nil {
		return err
	}

	FQDN := dnsName + "." + gc.baseDomain + "."

	// The resource record set to add.
	// Kind and SignatureRrdatas are set as
	// they are to satisfy reflect.DeepEqual.
	newRRSet := &gdnsv1.ResourceRecordSet{
		Kind:             "dns#resourceRecordSet",
		Name:             FQDN,
		Rrdatas:          svcIPs,
		SignatureRrdatas: []string{},
		Type:             "A",
		Ttl:              30,
	}

	clusterDNS, err := getClusterDNS(kclient)
	if err != nil {
		return err
	}

	var zones []configv1.DNSZone
	if clusterDNS.Spec.PublicZone != nil {
		zones = append(zones, *clusterDNS.Spec.PublicZone)
	}
	if clusterDNS.Spec.PrivateZone != nil {
		zones = append(zones, *clusterDNS.Spec.PrivateZone)
	}

	for _, zone := range zones {
		dnsChange := &gdnsv1.Change{
			Additions: []*gdnsv1.ResourceRecordSet{newRRSet},
		}

		// Look for an existing resource record set in the zone.
		listCall := gc.dnsService.ResourceRecordSets.List(gc.projectID, zone.ID)
		response, err := listCall.Name(FQDN).Do()
		if err != nil {
			return err
		}

		// There will be at most one result but loop anyway.
		// An empty slice will proceed directly to Create.
		for _, rrset := range response.Rrsets {
			if reflect.DeepEqual(newRRSet, rrset) {
				dnsChange.Additions = []*gdnsv1.ResourceRecordSet{}
			} else {
				dnsChange.Deletions = append(dnsChange.Deletions, rrset)
			}
		}

		if len(dnsChange.Additions) > 0 {
			log.Info("Submitting DNS changes:", "Zone", zone.ID,
				"Additions", dnsChange.Additions, "Deletions", dnsChange.Deletions)
			changesCall := gc.dnsService.Changes.Create(gc.projectID, zone.ID, dnsChange)
			_, err = changesCall.Do()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// Returns nil if forwarding rule is found for a given IP, or error if not found
func (gc *Client) ensureGCPForwardingRuleForExtIP(rhapiLbIP string) error {
	listCall := gc.computeService.ForwardingRules.List(gc.projectID, gc.region)
	response, err := listCall.Do()
	if err != nil {
		return err
	}

	for _, lb := range response.Items {
		if lb.IPAddress == rhapiLbIP {
			return nil
		}
	}
	return fmt.Errorf("Forwarding rule not found in GCP for given service IP %s", rhapiLbIP)

}

func (gc *Client) removeDNSForService(kclient k8s.Client, svc *corev1.Service, dnsName string) error {
	// google.golang.org/api/dns/v1.Service is a struct, not an interface, which
	// will make this all but impossible to write unit tests for
	FQDN := dnsName + "." + gc.baseDomain + "."

	clusterDNS, err := getClusterDNS(kclient)
	if err != nil {
		return err
	}

	var zones []configv1.DNSZone
	if clusterDNS.Spec.PublicZone != nil {
		zones = append(zones, *clusterDNS.Spec.PublicZone)
	}
	if clusterDNS.Spec.PrivateZone != nil {
		zones = append(zones, *clusterDNS.Spec.PrivateZone)
	}

	for _, zone := range zones {
		dnsChange := &gdnsv1.Change{}

		// Look for an existing resource record set in the zone.
		listCall := gc.dnsService.ResourceRecordSets.List(gc.projectID, zone.ID)
		response, err := listCall.Name(FQDN).Do()
		if err != nil {
			return err
		}

		// There will be at most one result but append anyway.
		dnsChange.Deletions = append(dnsChange.Deletions, response.Rrsets...)

		if len(dnsChange.Deletions) > 0 {
			log.Info("Submitting DNS changes:", "Zone", zone.ID, "Deletions", dnsChange.Deletions)
			call := gc.dnsService.Changes.Create(gc.projectID, zone.ID, dnsChange)
			_, err = call.Do()
			if err != nil {
				dnsError, ok := err.(*googleapi.Error)
				if !ok || dnsError.Code != http.StatusNotFound {
					return err
				}
			}
		}
	}

	return nil
}

func getIPAddressesFromService(svc *corev1.Service) ([]string, error) {
	var ips []string
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		ips = append(ips, ingress.IP)
	}

	if len(ips) == 0 {
		return nil, cioerrors.NewLoadBalancerNotReadyError()
	}

	return ips, nil
}

func (gc *Client) removeLoadBalancerFromMasterNodes(ctx context.Context, kclient k8s.Client) (string, error) {
	listCall := gc.computeService.ForwardingRules.List(gc.projectID, gc.region)
	response, err := listCall.Do()
	if err != nil {
		return "", err
	}

	extNLBName := gc.clusterName + "-api"
	intLBName := gc.clusterName + "-api-internal"
	var intIPAddress, lbName string
	for _, lb := range response.Items {
		// This list of forwardingrules (LBs) includes any service LBs
		// for application routers so check the port range and name to identify
		// the external API LB.
		if lb.LoadBalancingScheme == "EXTERNAL" && lb.PortRange == "6443-6443" && lb.Name == extNLBName {
			//delete the LB and remove it from the masters
			lbName = lb.Name
			_, err := gc.computeService.ForwardingRules.Delete(gc.projectID, gc.region, lbName).Do()
			if err != nil {
				return "", fmt.Errorf("Failed to delete ForwardingRule for external load balancer %v: %v", lb.Name, err)
			}
			err = removeGCPLBFromMasterMachines(kclient, lbName, gc.masterList)
			if err != nil {
				return "", err
			}
		}
		// we need this to update DNS
		if lb.LoadBalancingScheme == "INTERNAL" && lb.BackendService != "" && lb.Name == intLBName {
			// Unlike AWS, GCP NLBs don't have automatically assigned A records, just an external IP address
			// Save the internal NLB's IP Address in order to update the API's A record in the public DNS zone.
			intIPAddress = lb.IPAddress
		}
	}
	return intIPAddress, nil
}

func removeGCPLBFromMasterMachines(kclient k8s.Client, lbName string, masterNodes *machineapi.MachineList) error {
	for _, machine := range masterNodes.Items {
		providerSpecDecoded, err := getGCPDecodedProviderSpec(machine)
		if err != nil {
			log.Error(err, "Error retrieving decoded ProviderSpec for machine", "machine", machine.Name)
			return err
		}
		lbList := providerSpecDecoded.TargetPools
		newLBList := []string{}
		for _, lb := range lbList {
			if lb != lbName {
				log.Info("Machine's LB does not match LB to remove", "Machine LB", lb, "LB to remove", lbName)
				log.Info("Keeping machine's LB in machine object", "LB", lb, "Machine", machine.Name)
				newLBList = append(newLBList, lb)
			}
		}
		err = updateGCPLBList(kclient, lbList, newLBList, machine, providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error updating LB list for machine", "machine", machine.Name)
		}
	}
	return nil
}

func getGCPDecodedProviderSpec(machine machineapi.Machine) (*gcpproviderapi.GCPMachineProviderSpec, error) {
	s := runtime.NewScheme()
	err := gcpproviderapi.SchemeBuilder.AddToScheme(s)
	if err != nil {
		return nil, fmt.Errorf("Failed to register gcpproviderapi types to scheme: %v", err)
	}
	codecFactory := serializer.NewCodecFactory(s)
	decoder := codecFactory.UniversalDecoder(gcpproviderapi.SchemeGroupVersion)
	obj, gvk, err := decoder.Decode(machine.Spec.ProviderSpec.Value.Raw, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("Could not decode GCP ProviderSpec: %v", err)
	}
	spec, ok := obj.(*gcpproviderapi.GCPMachineProviderSpec)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

func encodeProviderSpec(in runtime.Object) (*runtime.RawExtension, error) {
	var buf bytes.Buffer
	s := runtime.NewScheme()
	err := gcpproviderapi.SchemeBuilder.AddToScheme(s)
	if err != nil {
		return nil, fmt.Errorf("Failed to register gcpproviderapi types to scheme: %v", err)
	}
	codecFactory := serializer.NewCodecFactory(s)
	serializerInfo := codecFactory.SupportedMediaTypes()
	encoder := codecFactory.EncoderForVersion(serializerInfo[0].Serializer, gcpproviderapi.SchemeGroupVersion)
	if err := encoder.Encode(in, &buf); err != nil {
		return nil, fmt.Errorf("Encoding ProviderSpec failed: %v", err)
	}
	return &runtime.RawExtension{Raw: buf.Bytes()}, nil
}

func updateGCPLBList(kclient k8s.Client, oldLBList []string, newLBList []string, machineToPatch machineapi.Machine, providerSpecDecoded *gcpproviderapi.GCPMachineProviderSpec) error {
	baseToPatch := k8s.MergeFrom(machineToPatch.DeepCopy())
	if !reflect.DeepEqual(oldLBList, newLBList) {
		providerSpecDecoded.TargetPools = newLBList
		newProviderSpecEncoded, err := encodeProviderSpec(providerSpecDecoded)
		if err != nil {
			log.Error(err, "Error encoding provider spec for machine", "machine", machineToPatch.Name)
			return err
		}
		machineToPatch.Spec.ProviderSpec.Value = newProviderSpecEncoded
		machineObj := machineToPatch.DeepCopy()
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

func (gc *Client) createExternalIP(name string, scheme string) (ipAddress string, err error) {
	// Check if an external IP with the correct name already exists
	addyList, err := gc.computeService.Addresses.List(gc.projectID, gc.region).Do()
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve list of GCP project's IP addresses: %v", err)
	}
	for _, ip := range addyList.Items {
		if ip.Name == name {
			log.Info("Static IP has already been reserved with the correct name. Reusing.", "Name", ip.Name, "IP Address", ip.Address)
			return ip.Address, nil
		}
	}
	// Create an external IP
	eip := &compute.Address{
		Name:        name,
		AddressType: scheme,
	}
	insertCall := gc.computeService.Addresses.Insert(gc.projectID, gc.region, eip)
	eipResp, err := insertCall.Do()
	if err != nil {
		return "", fmt.Errorf("Request to reserve a new static IP failed: %v", err)
	}

	waitResp, err := gc.computeService.RegionOperations.Wait(gc.projectID, gc.region, eipResp.Name).Do()

	// Fail if we couldn't reserve a static IP within 2 minutes.
	if waitResp.Status != "DONE" {
		return "", fmt.Errorf("Failed to reserve a static IP after waiting 120s: %v", err)
	}

	getCall := gc.computeService.Addresses.Get(gc.projectID, gc.region, name)
	address, err := getCall.Do()
	if err != nil {
		return "", err
	}
	log.Info("Reserved a new static IP for external load balancer", "IP address", address.Address)
	return address.Address, nil
}

func (gc *Client) releaseExternalIP(addressName string) error {
	_, err := gc.computeService.Addresses.Delete(gc.projectID, gc.region, addressName).Do()
	if err != nil {
		return fmt.Errorf("Failed to release External IP %v: %v", addressName, err)
	}
	return nil
}

func (gc *Client) createNetworkLoadBalancer(name string, scheme string, targetPool string, ip string) error {
	//Confirm the target pool is present and get its selflink URL
	tpResp, err := gc.computeService.TargetPools.Get(gc.projectID, gc.region, targetPool).Do()
	if err != nil {
		return fmt.Errorf("Unable to find expected targetPool %v: %v", targetPool, err)
	}
	tpURL := tpResp.SelfLink
	i := &compute.ForwardingRule{
		IPAddress:           ip,
		Name:                name,
		LoadBalancingScheme: scheme,
		NetworkTier:         "PREMIUM",
		Target:              tpURL,
		PortRange:           "6443-6443",
		IPProtocol:          "TCP",
	}
	_, err = gc.computeService.ForwardingRules.Insert(gc.projectID, gc.region, i).Do()
	if err != nil {
		return fmt.Errorf("Failed to create new ForwardingRule for %v: %v", name, err)
	}
	log.Info("Successfully created new ForwardingRule", "Name", name)
	return nil
}

func (gc *Client) updateAPIARecord(kclient k8s.Client, recordName string, newIP string) (oldIP string, err error) {
	clusterDNS, err := getClusterDNS(kclient)
	if err != nil {
		return "", err
	}
	pubZoneRecords, err := gc.dnsService.ResourceRecordSets.List(gc.projectID, clusterDNS.Spec.PublicZone.ID).Do()
	if err != nil {
		return "", fmt.Errorf("Failed to retrieve list of ResourceRecordSets from public zone %v : %v", clusterDNS.Spec.PublicZone.ID, err)
	}
	apiRRSets := []*gdnsv1.ResourceRecordSet{}
	for _, rrset := range pubZoneRecords.Rrsets {
		if rrset.Name == recordName {
			apiRRSets = append(apiRRSets, rrset)
		}
	}
	if len(apiRRSets) != 1 {
		return "", fmt.Errorf("Expected to find 1 A record for API, found %d", len(apiRRSets))
	}
	oldIP = apiRRSets[0].Rrdatas[0]
	if oldIP == newIP {
		// A record is already pointing to the correct IP, nothing to do
		log.Info("Default API A record is already pointing to the correct IP. No update necessary.", "IP address", newIP)
		return oldIP, nil
	}
	dnsChange := &gdnsv1.Change{}
	dnsChange.Deletions = append(dnsChange.Deletions, apiRRSets[0])
	updatedRRSet := *apiRRSets[0]
	updatedRRSet.Rrdatas = []string{newIP}
	dnsChange.Additions = append(dnsChange.Additions, &updatedRRSet)
	changesCall := gc.dnsService.Changes.Create(gc.projectID, clusterDNS.Spec.PublicZone.ID, dnsChange)
	_, err = changesCall.Do()
	if err != nil {
		return "", err
	}

	return oldIP, nil
}

func getClusterDNS(kclient k8s.Client) (*configv1.DNS, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "config.openshift.io/v1",
		Kind:    "dns",
	})
	ns := types.NamespacedName{
		Namespace: "",
		Name:      "cluster",
	}
	err := kclient.Get(context.TODO(), ns, u)
	if err != nil {
		return nil, err
	}

	uContent := u.UnstructuredContent()
	var dns *configv1.DNS
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(uContent, &dns)
	if err != nil {
		return nil, err
	}

	return dns, nil
}
