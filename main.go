/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/openshift/cloud-ingress-operator/config"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
	"github.com/openshift/cloud-ingress-operator/pkg/localmetrics"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	osdmetrics "github.com/openshift/operator-custom-metrics/pkg/metrics"
	"github.com/operator-framework/operator-lib/leader"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	apiv1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	apischemecontroller "github.com/openshift/cloud-ingress-operator/controllers/apischeme"
	publishingstrategycontroller "github.com/openshift/cloud-ingress-operator/controllers/publishingstrategy"
	routerservicecontroller "github.com/openshift/cloud-ingress-operator/controllers/routerservice"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

var (
	osdMetricsPort = "8181"
	osdMetricsPath = "/metrics"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiv1alpha1.AddToScheme(scheme))
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(machinev1beta1.Install(scheme))
	utilruntime.Must(ingresscontroller.AddToScheme(scheme))
	utilruntime.Must(machinev1.AddToScheme(scheme))
	scheme.AddKnownTypes(machinev1beta1.SchemeGroupVersion,
		&machinev1beta1.AWSMachineProviderConfig{},
	)
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8000", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: false,
		TimeEncoder: zapcore.RFC3339TimeEncoder,
		// Remove misleading controller-runtime stack traces https://github.com/kubernetes-sigs/kubebuilder/issues/1593
		StacktraceLevel: zapcore.DPanicLevel,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	watchNamespace, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get WatchNamespace,"+"the manager will watch and manage resources in all namespaces")
	}

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		Namespace:              watchNamespace,
	}

	if strings.Contains(watchNamespace, ",") {
		setupLog.Info("manager set up with multiple namespaces", "namespaces", watchNamespace)
		// nolint:golint,all
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(watchNamespace, ","))
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	if options.LeaderElection {
		err = leader.Become(ctx, "cloud-ingress-operator-lock")
		if err != nil {
			setupLog.Error(err, "failed to setup leader lock")
			os.Exit(1)
		}
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup Global Variables
	cli := mgr.GetClient()
	if err := baseutils.SetClusterVersion(cli); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	// setup apischemecontroller with mgr
	if err = (&apischemecontroller.APISchemeReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "APIScheme")
		os.Exit(1)
	}

	// setup publishingstrategycontroller with mgr
	if err = (&publishingstrategycontroller.PublishingStrategyReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PublishingStrategy")
		os.Exit(1)
	}

	// setup routerservice with mgr
	if err = (&routerservicecontroller.RouterServiceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RouterService")
		os.Exit(1)
	}

	addMetrics(ctx)

	//+kubebuilder:scaffold:builder

	if err = mgr.AddHealthzCheck("healthz", func(req *http.Request) error {
		kubeCli := mgr.GetClient()
		cloudPlatform, err := baseutils.GetPlatformType(kubeCli)
		if err != nil {
			return err
		}
		cloudClient := cloudclient.GetClientFor(kubeCli, *cloudPlatform)
		if err := cloudClient.Healthcheck(context.TODO(), kubeCli); err != nil {
			return err
		}
		return baseutils.SAhealthcheck(kubeCli)
	}); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err = mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// addMetrics will create the Services and Service Monitors to allow the operator export the metrics by using
// the Prometheus operator
func addMetrics(ctx context.Context) {
	metricsServer := osdmetrics.NewBuilder(config.OperatorNamespace, config.OperatorName).
		WithPort(osdMetricsPort).
		WithPath(osdMetricsPath).
		WithCollectors(localmetrics.MetricsList).
		WithServiceMonitor().
		GetConfig()

	if err := osdmetrics.ConfigureMetrics(ctx, *metricsServer); err != nil {
		setupLog.Error(err, "Failed to configure OSD metrics")
	}
}

func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}
