package controller

import (
	"github.com/openshift/cloud-ingress-operator/pkg/controller/APIScheme"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, APIScheme.Add)
}
