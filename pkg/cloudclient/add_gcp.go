package cloudclient

import (
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	Register(
		gcp.ClientIdentifier,
		func(kclient client.Client) CloudClient { return gcp.NewClient(kclient) },
	)
}
