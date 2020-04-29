package localmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("localmetrics")

var (
	MetricDefaultIngressController = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloud_ingress_operator_default_ingress",
		Help: "Report if default ingress is on cluster",
	})

	MetricsList = []prometheus.Collector{
		MetricDefaultIngressController,
	}
)
