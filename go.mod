module github.com/openshift/cloud-ingress-operator

go 1.15

require (
	github.com/aws/aws-sdk-go v1.38.38
	github.com/coreos/prometheus-operator v0.38.1-0.20200424145508-7e176fda06cc
	github.com/go-logr/logr v1.2.0
	github.com/golang/mock v1.4.4
	github.com/hashicorp/go-version v1.4.0
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/cluster-api-provider-gcp v0.0.0
	github.com/openshift/machine-api-operator v0.2.1-0.20200226185612-9b0170a1ba07
	github.com/openshift/operator-custom-metrics v0.4.2
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/prometheus/client_golang v1.7.1
	github.com/spf13/pflag v1.0.5
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43
	google.golang.org/api v0.35.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.23.0
	k8s.io/apimachinery v0.23.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	k8s.io/utils v0.0.0-20210802155522-efc7438f0176
	sigs.k8s.io/cluster-api-provider-aws v0.0.0
	sigs.k8s.io/controller-runtime v0.8.3
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.5.0
	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2 // Required by prometheus-operator
)

replace sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20200204144622-0df2d100309c // Pin OpenShift fork

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20220124143425-d74727069f6f
