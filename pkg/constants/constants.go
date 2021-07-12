package constants

const (
	// ProtectedDeleteAnnotation is an annotation used on ClusterDeployments to indicate that the ClusterDeployment
	// cannot be deleted. The annotation must be removed in order to delete the ClusterDeployment.
	ProtectedDeleteAnnotation = "cloudingress.openshift.io/protected-delete"
)
