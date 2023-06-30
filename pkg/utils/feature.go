package utils

var ClusterLegacyIngressLabel = "ext-managed.openshift.io/legacy-ingress-support"

func HasUserManagedIngressFeature(labels map[string]string) bool {
	if labels == nil {
		return false
	}
	// Not having the label is assumed to mean that the deployment is a legacy cluster
	legacyIngressLabel, labelExists := labels[ClusterLegacyIngressLabel]
	return labelExists && legacyIngressLabel == "false"
}
