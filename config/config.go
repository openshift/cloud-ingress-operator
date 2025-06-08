package config

const (
	// AdminAPIName is the name of the API endpoint for non-customer use (eg Hive)
	AdminAPIName string = "rh-api"

	// AdminAPISecurityGroupName is the name of the Security Group for the Admin API
	AdminAPISecurityGroupName string = "rh-api"

	// CloudAdminAPILoadBalancerName is the cloud provider identifier for the load
	// balancer for admin API endpoint
	CloudAdminAPILoadBalancerName string = "rh-api"

	// CustomerAPIName is the name of the API endpoint for customer use
	CustomerAPIName string = "api"

	// ExternalCloudAPILBNameSuffix is the cloud provider name suffix (eg aext, ext,
	// aint) for the default external API load balancer. This is not used by
	// AdminAPIName
	ExternalCloudAPILBNameSuffix string = "ext"

	// InternalCloudAPILBNameSuffix is the cloud provider name suffix (eg aext, ext,
	// aint) for the default internal API load balancer.
	InternalCloudAPILBNameSuffix string = "int"

	// InternalServicesTargetGroupSuffix internal services target group suffix
	InternalServicesTargetGroupSuffix string = "sint"
	// InternalAPITargetGroupSuffix internal api target group suffix
	InternalAPITargetGroupSuffix string = "aint"
	// ExternalAPITargetGroupSuffix external api target group suffix
	ExternalAPITargetGroupSuffix string = "aext"

	// OperatorName is the name of this operator
	OperatorName string = "cloud-ingress-operator"

	// KubeConfigNamespace is where to find the cluster-config
	KubeConfigNamespace string = "kube-system"

	// KubeConfigConfigMapName is the config blob for the cluster, containing region
	// availability zone, networking information, base domain, cluster name and more
	KubeConfigConfigMapName string = "cluster-config-v1"

	// AdminAPIListenerPort
	AdminAPIListenerPort int64 = 6443

	// MaxAPIRetries
	MaxAPIRetries int = 10

	// AWSSecretName
	AWSSecretName string = "cloud-ingress-operator-credentials-aws" //#nosec G101 -- This is a false positive

	// GCPSecretName
	GCPSecretName string = "cloud-ingress-operator-credentials-gcp" //#nosec G101 -- This is a false positive

	// OperatorNamespace
	OperatorNamespace string = "openshift-cloud-ingress-operator"

	// olm.skipRange annotation added to CSV --SREP-96
	EnableOLMSkipRange string = "true"
)
