package controller

import (
	"github.com/openshift/cloud-ingress-operator/pkg/controller/apischeme"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, apischeme.Add)
}
