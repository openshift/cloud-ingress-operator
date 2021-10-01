package cloudclient

import (
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	Register(
		gcp.ClientIdentifier,
		produceGCP,
	)
}

func produceGCP(kclient client.Client) CloudClient {
	cli, err := gcp.NewClient(kclient)
	if err != nil {
		panic(err)
	}

	return cli
}
