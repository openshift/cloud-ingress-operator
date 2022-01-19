package main

import (
	"flag"
	"os" // TODO remove me, shouldn't be necessary after logging is added. ZAP

	"github.com/openshift/cloud-ingress-operator/pkg/apis"
	ciovalidatingwebhooks "github.com/openshift/cloud-ingress-operator/pkg/validating-webhooks"
	admissionCmd "github.com/openshift/generic-admission-server/pkg/cmd"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

/*
var (
	tlsKey  string
	tlsCert string
	caCert  string
)
*/

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	/*
		// Service cert flags
		pflag.StringVar("tslKey", "", tlsKey, "TLS key for TLS")
		pflag.StringVar("tlsCert", "", tlsCert, "TLS Certificate")
		pflag.StringVar("caCert", "", caCert, "CA Cert File")
	*/

	pflag.Parse()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())
	decoder := createDecoder()

	admissionCmd.RunAdmissionServer(
		ciovalidatingwebhooks.NewAPISchemeValidatingAdmissionHook(decoder),
		ciovalidatingwebhooks.NewSSHDValidatingAdmissionHook(decoder),
	)
}

func createDecoder() *admission.Decoder {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	decoder, err := admission.NewDecoder(scheme)
	if err != nil {
		// TODO logging
		// log.Fatalf("Count not create decoder for admission hooks")
		os.Exit(1)
	}
	return decoder
}
