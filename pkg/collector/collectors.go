package collector

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCloudIngressCollectorList(c client.Client) []prometheus.Collector {
	return []prometheus.Collector{
		MetricDefaultIngressController,
		&CloudIngressCollector{
			client: c,
		},
	}
}
