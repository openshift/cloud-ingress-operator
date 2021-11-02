package main

import (
	"github.com/openshift/generic-admission-server/pkg/cmd"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ciov1 "github.com/openshift/cloud-ingress-operator/pkg/apis"
	ciowebhook "github.com/openshift/cloud-ingress-operator/pkg/cio-webhook"
	"github.com/openshift/cloud-ingress-operator/version"
)

func main() {
	log.Infof("Version: %s", version.String())
	log.Info("Starting CRD Validation Webhooks.")

	decoder := createDecoder()
	cmd.RunAdmissionServer(
		ciowebhook.NewApischemeDeleteAdmissionHook(decoder),
	)
}

func createDecoder() *admission.Decoder {
	scheme := runtime.NewScheme()
	ciov1.AddToScheme(scheme)
	decoder, err := admission.NewDecoder(scheme)
	if err != nil {
		log.WithError(err).Fatal("could not create a decoder")
	}
	return decoder
}
