package cloudclient

import (
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient/aws"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	Register(
		aws.ClientIdentifier,
		func(kclient client.Client) CloudClient { return aws.NewClient(kclient) },
	)
}
