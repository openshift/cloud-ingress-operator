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
	"net/http"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cloud-ingress-operator/pkg/cloudclient"
	"github.com/openshift/cloud-ingress-operator/pkg/config"
	"github.com/openshift/cloud-ingress-operator/pkg/ingresscontroller"
	"github.com/openshift/cloud-ingress-operator/pkg/localmetrics"
	baseutils "github.com/openshift/cloud-ingress-operator/pkg/utils"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	osdmetrics "github.com/openshift/operator-custom-metrics/pkg/metrics"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	awsproviderapi "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	cloudingressmanagedopenshiftiov1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	apischemecontroller "github.com/openshift/cloud-ingress-operator/controllers/apischeme"
	publishingstrategycontroller "github.com/openshift/cloud-ingress-operator/controllers/publishingstrategy"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

var (
	osdMetricsPort    = "8181"
	osdMetricsPath    = "/metrics"
	livenessProbePort = "8000"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(cloudingressmanagedopenshiftiov1alpha1.AddToScheme(scheme))

	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(machineapi.AddToScheme(scheme))
	utilruntime.Must(awsproviderapi.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(ingresscontroller.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f3d6b689.cloudingress.managed.openshift.io",
		Namespace:              config.OperatorNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
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

	if err := monitoringv1.AddToScheme(mgr.GetScheme()); err != nil {
		setupLog.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()
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
