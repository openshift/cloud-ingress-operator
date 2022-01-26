module github.com/openshift/cloud-ingress-operator

go 1.15

require (
	github.com/aws/aws-sdk-go v1.38.38
	github.com/coreos/prometheus-operator v0.38.1-0.20200424145508-7e176fda06cc
	github.com/go-logr/logr v0.4.0
	github.com/go-openapi/spec v0.19.5
	github.com/golang/mock v1.4.4
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/cluster-api-provider-gcp v0.0.0
	github.com/openshift/generic-admission-server v1.14.1-0.20200903115324-4ddcdd976480
	github.com/openshift/machine-api-operator v0.2.1-0.20200226185612-9b0170a1ba07
	github.com/openshift/operator-custom-metrics v0.4.2
	github.com/operator-framework/operator-sdk v0.18.2
	github.com/prometheus/client_golang v1.11.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43
	google.golang.org/api v0.35.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.0
	k8s.io/apimachinery v0.22.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	k8s.io/utils v0.0.0-20210707171843-4b05e18ac7d9
	sigs.k8s.io/cluster-api-provider-aws v0.0.0
	sigs.k8s.io/controller-runtime v0.9.0
)

replace (
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v13.3.2+incompatible // Required by OLM
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.5.0
	k8s.io/api => k8s.io/api v0.22.0
	k8s.io/apimachinery => k8s.io/apimachinery v0.22.0
	k8s.io/apiserver => k8s.io/apiserver v0.22.0
	k8s.io/client-go => k8s.io/client-go v0.22.0 // Required by prometheus-operator
)

replace sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20200204144622-0df2d100309c // Pin OpenShift fork
