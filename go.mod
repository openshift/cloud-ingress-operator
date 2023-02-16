module github.com/openshift/cloud-ingress-operator

go 1.17

require (
	github.com/aws/aws-sdk-go v1.44.199
	github.com/go-logr/logr v1.2.3
	github.com/golang/mock v1.6.0
	github.com/hashicorp/go-version v1.6.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.8.1
	github.com/onsi/gomega v1.26.0
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/cluster-api-provider-gcp v0.0.0
	github.com/openshift/operator-custom-metrics v0.5.0
	github.com/openshift/osde2e v0.0.0-20230214212541-6d477568d9ae
	github.com/operator-framework/operator-lib v0.11.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.61.0
	github.com/stretchr/testify v1.8.1
	go.uber.org/zap v1.24.0
	google.golang.org/api v0.109.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.26.1
	k8s.io/apimachinery v0.26.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20221110221610-a28e98eb7c70
	k8s.io/utils v0.0.0-20221128185143-99ec85e7a448
	sigs.k8s.io/cluster-api-provider-aws v0.0.0
	sigs.k8s.io/controller-runtime v0.14.1
)

require (
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	github.com/AlecAivazis/survey/v2 v2.3.2 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/adamliesko/retry v0.0.0-20200123222335-86c8baac277d // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/briandowns/spinner v1.11.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.0 // indirect
	github.com/emicklei/go-restful/v3 v3.10.0 // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fatih/color v1.14.1 // indirect
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32 // indirect
	github.com/golang-jwt/jwt/v4 v4.4.1 // indirect
	github.com/golang/glog v1.0.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/gorilla/websocket v1.4.2 // indirect
	github.com/h2non/filetype v1.1.1 // indirect
	github.com/h2non/go-is-svg v0.0.0-20160927212452-35e8c4b0612c // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/joshdk/go-junit v1.0.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/microcosm-cc/bluemonday v1.0.18 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/spdystream v0.2.0 // indirect
	github.com/openshift-online/ocm-sdk-go v0.1.316 // indirect
	github.com/openshift/client-go v0.0.0-20220603133046-984ee5ebedcf // indirect
	github.com/openshift/cloud-credential-operator v0.0.0-20230125000001-006e1c8ea7a1 // indirect
	github.com/openshift/custom-domains-operator v0.0.0-20221118201157-bd1052dac818 // indirect
	github.com/openshift/machine-api-operator v0.2.1-0.20191128180243-986b771e661d // indirect
	github.com/openshift/managed-upgrade-operator v0.0.0-20230128000023-b30cca4aa2af // indirect
	github.com/openshift/must-gather-operator v0.1.2-0.20221011152618-7805956e1ded // indirect
	github.com/openshift/rosa v1.2.14 // indirect
	github.com/openshift/route-monitor-operator v0.0.0-20221118160357-3df1ed1fa1d2 // indirect
	github.com/openshift/splunk-forwarder-operator v0.0.0-20230125212852-176d68d8c59b // indirect
	github.com/operator-framework/api v0.17.2-0.20220915200120-ff2dbc53d381 // indirect
	github.com/operator-framework/operator-lifecycle-manager v0.22.0 // indirect
	github.com/operator-framework/operator-registry v1.26.3 // indirect
	github.com/pelletier/go-toml/v2 v2.0.6 // indirect
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.61.0 // indirect
	github.com/redhat-cop/must-gather-operator v1.1.2 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/slack-go/slack v0.12.1 // indirect
	github.com/spf13/afero v1.9.3 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/cobra v1.6.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.15.0 // indirect
	github.com/subosito/gotenv v1.4.2 // indirect
	github.com/vmware-tanzu/velero v1.7.2 // indirect
	github.com/zgalor/weberr v0.6.0 // indirect
	gitlab.com/c0b/go-ordered-json v0.0.0-20171130231205-49bbdab258c2 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	sigs.k8s.io/e2e-framework v0.1.0 // indirect
)

require (
	cloud.google.com/go/compute v1.15.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logr/zapr v1.2.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.1 // indirect
	github.com/googleapis/gax-go/v2 v2.7.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_golang v1.14.0
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.39.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/net v0.6.0 // indirect
	golang.org/x/oauth2 v0.5.0
	golang.org/x/sys v0.5.0 // indirect
	golang.org/x/term v0.5.0 // indirect
	golang.org/x/text v0.7.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.2.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/grpc v1.53.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiextensions-apiserver v0.26.0 // indirect
	k8s.io/component-base v0.26.0 // indirect
	k8s.io/klog v1.0.0 // indirect
	k8s.io/klog/v2 v2.80.1 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/openshift/api => github.com/openshift/api v0.0.0-20220826140015-b21e86c742e7
	k8s.io/apimachinery => k8s.io/apimachinery v0.26.0
	k8s.io/client-go => k8s.io/client-go v0.26.0
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20200204144622-0df2d100309c // Pin OpenShift fork
)
