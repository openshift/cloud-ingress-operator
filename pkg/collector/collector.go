package collector

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/cloud-ingress-operator/pkg/apis/cloudingress/v1alpha1"
)

var (
	APISchemesDesc = prometheus.NewDesc(
		"cloud_ingress_operator_apischemes",
		"List of APISchemes",
		[]string{
			"name",
			"namespace",
		}, nil)
)

// CloudIngressCollector is implementing prometheus.Collector interface.
type CloudIngressCollector struct {
	client client.Client
}

func NewCloudIngressCollector(c client.Client) prometheus.Collector {
	return &CloudIngressCollector{
		client: c,
	}
}

// Describe implements the prometheus.Collector interface.
func (cic *CloudIngressCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- APISchemesDesc
}

// Collect is method required to implement the prometheus.Collector(prometheus/client_golang/prometheus/collector.go) interface.
func (cic *CloudIngressCollector) Collect(ch chan<- prometheus.Metric) {
	cic.collectCloudIngressMetrics(ch)
}

func (cic *CloudIngressCollector) collectCloudIngressMetrics(ch chan<- prometheus.Metric) {
	apiList := &v1alpha1.APISchemeList{}
	err := cic.client.List(context.TODO(), apiList)
	if err != nil {
		return
	}

	for _, apis := range apiList.Items {
		ch <- prometheus.MustNewConstMetric(
			APISchemesDesc,
			prometheus.GaugeValue,
			float64(apis.CreationTimestamp.Unix()),
			apis.Name,
			apis.Namespace,
		)
	}
}
