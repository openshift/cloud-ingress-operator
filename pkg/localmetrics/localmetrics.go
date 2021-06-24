package localmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	MetricDefaultIngressController = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cloud_ingress_operator_default_ingress",
		Help: "Report if default ingress is on cluster",
	})
	MetricAPISchemeConditionStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "cloud_ingress_operator_apischeme_condition",
		Help: "Report the status of the APIScheme condition",
	}, []string{"status", "message", "reason"})

	MetricsList = []prometheus.Collector{
		MetricDefaultIngressController,
		MetricAPISchemeConditionStatus,
	}
)
