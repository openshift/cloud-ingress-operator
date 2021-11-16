package utils

import (
	"context"

	operv1 "github.com/openshift/api/operator/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SAhealthcheck will perform a basic call to make sure ingresscontrollers is reachable
// covers: https://github.com/openshift/cloud-ingress-operator/blob/32e50ef2aa8571f9bb60aaf53ed9d1262cc2c083/deploy/20_cloud-ingress-operator_openshift-ingress-operator.Role.yaml#L39-L50
func SAhealthcheck(kclient client.Client) error {
	var op operv1.IngressController
	ns := types.NamespacedName{
		Namespace: "openshift-ingress-operator",
		Name:      "default",
	}
	return kclient.Get(context.TODO(), ns, &op)
}
