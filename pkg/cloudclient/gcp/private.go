package gcp

// "Private" or non-interface conforming methods

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	gdnsv1 "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"

	configv1 "github.com/openshift/api/config/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cioerrors "github.com/openshift/cloud-ingress-operator/pkg/errors"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
)

// ensureAdminAPIDNS ensures the DNS record for the "admin API" Service
// LoadBalancer is accurately set
func (c *Client) ensureAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.ensureDNSForService(kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName)
}

// deleteAdminAPIDNS ensures the DNS record for the "admin API" Service
// LoadBalancer is deleted
func (c *Client) deleteAdminAPIDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.APIScheme, svc *corev1.Service) error {
	return c.removeDNSForService(kclient, svc, instance.Spec.ManagementAPIServerIngress.DNSName)
}

// ensureSSHDNS ensures the DNS record for the SSH Service LoadBalancer
// is accurately set
func (c *Client) ensureSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.ensureDNSForService(kclient, svc, instance.Spec.DNSName)
}

// deleteSSHDNS ensures the DNS record for the SSH Service LoadBalancer
// is deleted
func (c *Client) deleteSSHDNS(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.SSHD, svc *corev1.Service) error {
	return c.removeDNSForService(kclient, svc, instance.Spec.DNSName)
}

// setDefaultAPIPrivate sets the default api (api.<cluster-domain>) to private
// scope
func (c *Client) setDefaultAPIPrivate(ctx context.Context, kclient client.Client, _ *cloudingressv1alpha1.PublishingStrategy) error {
	return errors.New("setDefaultAPIPrivate is not implemented")
}

// setDefaultAPIPublic sets the default API (api.<cluster-domain>) to public
// scope
func (c *Client) setDefaultAPIPublic(ctx context.Context, kclient client.Client, instance *cloudingressv1alpha1.PublishingStrategy) error {
	return errors.New("setDefaultAPIPublic is not implemented")
}

func (c *Client) ensureDNSForService(kclient client.Client, svc *corev1.Service, dnsName string) error {
	// google.golang.org/api/dns/v1.Service is a struct, not an interface, which
	// will make this all but impossible to write unit tests for

	svcIPs, err := getIPAddressesFromService(svc)
	if err != nil {
		return err
	}

	baseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	FQDN := dnsName + "." + baseDomain + "."

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

	clusterDNS := &configv1.DNS{}
	err = kclient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, clusterDNS)
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
		listCall := c.dnsService.ResourceRecordSets.List(c.projectID, zone.ID)
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
			changesCall := c.dnsService.Changes.Create(c.projectID, zone.ID, dnsChange)
			_, err = changesCall.Do()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Client) removeDNSForService(kclient client.Client, svc *corev1.Service, dnsName string) error {
	// google.golang.org/api/dns/v1.Service is a struct, not an interface, which
	// will make this all but impossible to write unit tests for

	baseDomain, err := baseutils.GetClusterBaseDomain(kclient)
	if err != nil {
		return err
	}
	FQDN := dnsName + "." + baseDomain + "."

	clusterDNS := &configv1.DNS{}
	err = kclient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, clusterDNS)
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
		listCall := c.dnsService.ResourceRecordSets.List(c.projectID, zone.ID)
		response, err := listCall.Name(FQDN).Do()
		if err != nil {
			return err
		}

		// There will be at most one result but loop anyway.
		for _, rrset := range response.Rrsets {
			dnsChange.Deletions = append(dnsChange.Deletions, rrset)
		}

		if len(dnsChange.Deletions) > 0 {
			log.Info("Submitting DNS changes:", "Zone", zone.ID, "Deletions", dnsChange.Deletions)
			call := c.dnsService.Changes.Create(c.projectID, zone.ID, dnsChange)
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
