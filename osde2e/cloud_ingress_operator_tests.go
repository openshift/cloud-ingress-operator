// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS.
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	cloudingressv1alpha1 "github.com/openshift/cloud-ingress-operator/api/v1alpha1"
	"github.com/openshift/osde2e-common/pkg/clients/ocm"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"github.com/openshift/osde2e-common/pkg/clients/prometheus"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// TODO: resolve pending tests

var _ = ginkgo.Describe("cloud-ingress-operator", ginkgo.Ordered, func() {
	var (
		ocmClient     *ocm.Client
		oc            *openshift.Client
		prom          *prometheus.Client
		awsSession    *session.Session
		cloudProvider string
		name          = "cloud-ingress-operator"
		namespace     = "openshift-" + name
		apiSchemeName = "rh-api"
	)

	ginkgo.BeforeAll(func(ctx context.Context) {
		log.SetLogger(ginkgo.GinkgoLogr)

		clusterID := os.Getenv("CLUSTER_ID")
		Expect(clusterID).ShouldNot(BeEmpty(), "failed to find CLUSTER_ID environment variable")

		var err error
		oc, err = openshift.New(ginkgo.GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")
		Expect(cloudingressv1alpha1.AddToScheme(oc.GetScheme())).Should(Succeed(), "unable to register cloudingressv1alpha1 scheme")

		prom, err = prometheus.New(ctx, oc)
		Expect(err).ShouldNot(HaveOccurred(), "failed to create prometheus client")

		// TODO: support multiple ocm environments
		ocmClient, err = ocm.New(ctx, os.Getenv("OCM_TOKEN"), ocm.Stage)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup ocm client")
		ginkgo.DeferCleanup(ocmClient.Connection.Close)

		cluster, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().SendContext(ctx)
		Expect(err).ShouldNot(HaveOccurred(), "unable to get cluster %s", clusterID)

		if cluster.Body().AWS().STS().Enabled() || cluster.Body().Hypershift().Enabled() {
			ginkgo.Skip("cloud-ingress-operator is not deployed to clusters with STS or Hypershift enabled")
		}

		cloudProvider = cluster.Body().CloudProvider().ID()
		// TODO: get region
		_ = cluster.Body().Region().ID()

		switch cloudProvider {
		case "aws":
			// TODO: load aws creds and create client
			awsCfg := aws.NewConfig()
			awsSession, err = session.NewSession(awsCfg)
			Expect(err).ShouldNot(HaveOccurred(), "unable to create aws client")
		case "gcp":
			// TODO: load gcp creds and create client
		default:
			panic("wtf")
		}
	})

	ginkgo.It("is installed", func(ctx context.Context) {
		ginkgo.By("checking the namespace exists")
		err := oc.Get(ctx, namespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", namespace)

		ginkgo.By("checking the role exists")
		var roles rbacv1.RoleList
		err = oc.WithNamespace(namespace).List(ctx, &roles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list roles")
		Expect(&roles).Should(ContainItemWithPrefix(name), "unable to find roles with prefix %s", name)

		ginkgo.By("checking the rolebinding exists")
		var rolebindings rbacv1.RoleBindingList
		err = oc.List(ctx, &rolebindings)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list rolebindings")
		Expect(&rolebindings).Should(ContainItemWithPrefix(name), "unable to find rolebindings with prefix %s", name)

		ginkgo.By("checking the clusterrole exists")
		var clusterRoles rbacv1.ClusterRoleList
		err = oc.List(ctx, &clusterRoles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list clusterroles")
		Expect(&clusterRoles).Should(ContainItemWithPrefix(name), "unable to find cluster role with prefix %s", name)

		ginkgo.By("checking the clusterrolebinding exists")
		var clusterRoleBindings rbacv1.ClusterRoleBindingList
		err = oc.List(ctx, &clusterRoleBindings)
		Expect(err).ShouldNot(HaveOccurred(), "unable to list clusterrolebindings")
		Expect(&clusterRoleBindings).Should(ContainItemWithPrefix(name), "unable to find clusterrolebinding with prefix %s", name)

		ginkgo.By("checking the service exists")
		err = oc.Get(ctx, name, namespace, &corev1.Service{})
		Expect(err).ShouldNot(HaveOccurred(), "service %s/%s not found", namespace, name)

		ginkgo.By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, oc, name, namespace).Should(BeAvailable())

		ginkgo.By("checking the configmap lockfile exists")
		EventuallyConfigMap(ctx, oc, name+"-lock", namespace).ShouldNot(BeNil())

		ginkgo.By("checking metrics are being exported")
		results, err := prom.InstantQuery(ctx, `up{job="cloud-ingress-operator"}`)
		Expect(err).ShouldNot(HaveOccurred(), "failed to query prometheus")
		Expect(int(results[0].Value)).Should(BeNumerically("==", 1), "prometheus exporter is not healthy")
	})

	// ginkgo.It("can be upgraded", ...)

	ginkgo.Describe("APIScheme", func() {
		apischeme := &cloudingressv1alpha1.APIScheme{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "osde2e-",
			},
			Spec: cloudingressv1alpha1.APISchemeSpec{
				ManagementAPIServerIngress: cloudingressv1alpha1.ManagementAPIServerIngress{
					DNSName: "osde2e",
				},
			},
		}

		ginkgo.It("can't be managed by dedicated-admins group", func(ctx context.Context) {
			dedicatedAdmin, err := oc.Impersonate("test-user", "dedicated-admins")
			Expect(err).ShouldNot(HaveOccurred(), "unable to impersonate dedicated-admins group")

			err = dedicatedAdmin.Create(ctx, apischeme)
			Expect(apierrors.IsForbidden(err)).Should(BeTrue(), "expected forbidden err, got %v", err)
		})

		ginkgo.It("can be managed by cluster admin", func(ctx context.Context) {
			Expect(oc.Create(ctx, apischeme)).Should(Succeed(), "unable to create APIScheme CR")
			Expect(oc.Delete(ctx, apischeme)).Should(Succeed(), "unable to delete APIScheme CR")
		})

		ginkgo.It("AllowedCIDRBlocks changes are updated on the Service", func(ctx context.Context) {
			ginkgo.By(fmt.Sprintf("getting the %s/%s APIScheme", namespace, apiSchemeName))
			scheme := new(cloudingressv1alpha1.APIScheme)
			Expect(oc.Get(ctx, apiSchemeName, namespace, scheme)).Should(Succeed(), "unable to get %s APIScheme", apiSchemeName)

			ginkgo.By("removing the last CIDRBlock in the AllowedCIDRBlocks list")
			allowedCIDRBlocks := scheme.Spec.ManagementAPIServerIngress.AllowedCIDRBlocks
			newAllowedCIDRBlocks := allowedCIDRBlocks[:len(allowedCIDRBlocks)-1]
			scheme.Spec.ManagementAPIServerIngress.AllowedCIDRBlocks = newAllowedCIDRBlocks

			ginkgo.By("updating the APIScheme")
			Expect(oc.Update(ctx, scheme)).Should(Succeed(), "failed to update APIScheme")

			ginkgo.By("waiting for the change to be reflected in the Service")
			Eventually(ctx, func(ctx context.Context) (bool, error) {
				service := new(corev1.Service)
				if err := oc.Get(ctx, apiSchemeName, "openshift-kube-apiserver", service); err != nil {
					return false, err
				}
				return reflect.DeepEqual(service.Spec.LoadBalancerSourceRanges, newAllowedCIDRBlocks), nil
			})

			scheme.Spec.ManagementAPIServerIngress.AllowedCIDRBlocks = allowedCIDRBlocks
			ginkgo.By("updating the APIScheme with the original list")
			Expect(oc.Update(ctx, scheme)).Should(Succeed(), "failed to update APIScheme")

			ginkgo.By("waiting for the change to be reflected in the Service")
			Eventually(ctx, func(ctx context.Context) (bool, error) {
				service := new(corev1.Service)
				if err := oc.Get(ctx, apiSchemeName, "openshift-kube-apiserver", service); err != nil {
					return false, err
				}
				return reflect.DeepEqual(service.Spec.LoadBalancerSourceRanges, allowedCIDRBlocks), nil
			})
		})

		ginkgo.It("hostname can be resolved", func(ctx context.Context) {
			apiserver := new(configv1.APIServer)
			Expect(oc.Get(ctx, "cluster", "", apiserver)).Should(Succeed(), "unable to get APIServer")

			var hostname string
			for _, namedCertificate := range apiserver.Spec.ServingCerts.NamedCertificates {
				for _, name := range namedCertificate.Names {
					if strings.HasPrefix(name, "rh-api") {
						hostname = name
					}
				}
			}

			Eventually(func() error {
				_, err := net.LookupHost(hostname)
				return err
			}).Should(Succeed(), "failed to lookup rh-api hostname")
		})

		ginkgo.PIt("external LoadBalancer is restored upon deletion", func(ctx context.Context) {
			// get the LB name from the service
			getLoadBalancerName := func(ctx context.Context) (string, error) {
				service := new(corev1.Service)
				if err := oc.Get(ctx, apiSchemeName, "openshift-kube-apiserver", service); err != nil {
					return "", err
				}
				if cloudProvider == "gcp" {
					return service.Status.LoadBalancer.Ingress[0].IP, nil
				}
				return service.Status.LoadBalancer.Ingress[0].Hostname[0:32], nil
			}

			lbName, err := getLoadBalancerName(ctx)
			Expect(err).ShouldNot(HaveOccurred(), "failed to get service name")

			switch cloudProvider {
			case "aws":
				elbclient := elb.New(awsSession)

				oldLBDesc, err := elbclient.DescribeLoadBalancersWithContext(ctx, &elb.DescribeLoadBalancersInput{LoadBalancerNames: aws.StringSlice([]string{lbName})})
				Expect(err).ShouldNot(HaveOccurred(), "unabled to list loadbalancers")
				Expect(elbclient.DeleteLoadBalancer(&elb.DeleteLoadBalancerInput{LoadBalancerName: aws.String(lbName)})).Should(Succeed())

				ginkgo.DeferCleanup(func(ctx context.Context) {
					ec2client := ec2.New(awsSession)
					for _, groupID := range oldLBDesc.LoadBalancerDescriptions[0].SecurityGroups {
						// revoke egress/ingress stuff
						_, err := ec2client.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{GroupId: groupID})
						Expect(err).ShouldNot(HaveOccurred(), "failed to delete security group")
					}
				})
			case "gcp":
				// defer cleanup the old security groups
				ginkgo.DeferCleanup(func(ctx context.Context) {
				})
			default:
				panic("whoops not supported")
			}

			ginkgo.By("waiting for the LoadBalancer to be recreated in the cloud provider")
			Eventually(ctx, func(ctx context.Context) (bool, error) {
				newLBName, err := getLoadBalancerName(ctx)
				if err != nil {
					if apierrors.IsNotFound(err) {
						return false, nil
					}
					return false, err
				}
				return newLBName != lbName, nil
			})
		})
	})

	ginkgo.Describe("PublishingStrategies", func() {
		var publishingstrategy cloudingressv1alpha1.PublishingStrategy

		ginkgo.BeforeEach(func(ctx context.Context) {
			ginkgo.By("getting the default PublishingStrategy")
			err := oc.Get(ctx, "publishingstrategy", namespace, &publishingstrategy)
			Expect(err).ShouldNot(HaveOccurred(), "unable to get default publishingstrategy")
		})

		ginkgo.It("can't be managed by dedicated-admins group", func(ctx context.Context) {
			publishingstrategy := &cloudingressv1alpha1.PublishingStrategy{ObjectMeta: metav1.ObjectMeta{GenerateName: "osde2e-", Namespace: "default"}}
			dedicatedAdmin, err := oc.Impersonate("test-user", "dedicated-admins")
			Expect(err).ShouldNot(HaveOccurred(), "unable to impersonate dedicated-admins group")

			err = dedicatedAdmin.Create(ctx, publishingstrategy)
			Expect(apierrors.IsForbidden(err)).Should(BeTrue(), "expected forbidden err, got %v", err)
		})

		ginkgo.It("DNSName changes are propogated to the IngressController", func(ctx context.Context) {
			ginkgo.By("getting the default IngressController")
			ingresscontroller := new(operatorv1.IngressController)
			err := oc.Get(ctx, "default", "openshift-ingress-operator", ingresscontroller)
			Expect(err).ShouldNot(HaveOccurred(), "unable to get ingresscontroller")

			originalDNSName := ingresscontroller.Spec.Domain
			newDNSName := "osde2e." + originalDNSName

			ingressControllerDomainCanBeUpdated := func(ctx context.Context, name string) {
				ginkgo.GinkgoHelper()

				ginkgo.By(fmt.Sprintf("updating the PublishingStrategy's default ApplicationIngress DNSName to %q", name))
				for i := range publishingstrategy.Spec.ApplicationIngress {
					if publishingstrategy.Spec.ApplicationIngress[i].Default {
						publishingstrategy.Spec.ApplicationIngress[i].DNSName = name
					}
				}

				err = oc.Update(ctx, &publishingstrategy)
				Expect(err).ShouldNot(HaveOccurred(), "unable to update publishingstrategy with new dnsname")

				ginkgo.By("waiting for the change to be reflected in openshift-ingress-operator/default IngressController's domain")
				Eventually(ctx, func(ctx context.Context) (bool, error) {
					err := oc.Get(ctx, "default", "openshift-ingress-operator", ingresscontroller)
					if err != nil {
						return false, err
					}
					return ingresscontroller.Spec.Domain == name, nil
				})
			}

			ingressControllerDomainCanBeUpdated(ctx, newDNSName)
			ingressControllerDomainCanBeUpdated(ctx, originalDNSName)
		})

		ginkgo.It("ApplicationIngress.RouteSelector is propogated to the IngressController", func(ctx context.Context) {
			expectedMatchExpressions := []metav1.LabelSelectorRequirement{
				{
					Key:      "osde2e",
					Operator: metav1.LabelSelectorOperator("in"),
					Values:   []string{"here"},
				},
			}

			for i := range publishingstrategy.Spec.ApplicationIngress {
				if publishingstrategy.Spec.ApplicationIngress[i].Default {
					publishingstrategy.Spec.ApplicationIngress[i].RouteSelector.MatchLabels = map[string]string{"osde2e": "here"}
					publishingstrategy.Spec.ApplicationIngress[i].RouteSelector.MatchExpressions = expectedMatchExpressions
				}
			}
			Expect(oc.Update(ctx, &publishingstrategy)).Should(Succeed(), "failed to update publishingstrategy")

			Eventually(ctx, func(ctx context.Context) (bool, error) {
				ingresscontroller := new(operatorv1.IngressController)
				if err := oc.Get(ctx, "default", "openshift-ingress-operator", ingresscontroller); err != nil {
					return false, err
				}
				_, ok := ingresscontroller.Spec.RouteSelector.MatchLabels["osde2e"]
				if !ok {
					return false, nil
				}
				return reflect.DeepEqual(ingresscontroller.Spec.RouteSelector.MatchExpressions, expectedMatchExpressions), nil
			})

			for i := range publishingstrategy.Spec.ApplicationIngress {
				if publishingstrategy.Spec.ApplicationIngress[i].Default {
					publishingstrategy.Spec.ApplicationIngress[i].RouteSelector.MatchLabels = map[string]string{}
					publishingstrategy.Spec.ApplicationIngress[i].RouteSelector.MatchExpressions = []metav1.LabelSelectorRequirement{}
				}
			}
			Expect(oc.Update(ctx, &publishingstrategy)).Should(Succeed(), "failed to update publishingstrategy")

			Eventually(ctx, func(ctx context.Context) (bool, error) {
				ingresscontroller := new(operatorv1.IngressController)
				if err := oc.Get(ctx, "default", "openshift-ingress-operator", ingresscontroller); err != nil {
					return false, err
				}
				return len(ingresscontroller.Spec.RouteSelector.MatchLabels) == 0 && len(ingresscontroller.Spec.RouteSelector.MatchExpressions) == 0, nil
			})
		})

		ginkgo.It("Certificate changes are propogated to the IngressController", func(ctx context.Context) {
			ginkgo.By("getting the default IngressController")
			ingresscontroller := new(operatorv1.IngressController)
			err := oc.Get(ctx, "default", "openshift-ingress-operator", ingresscontroller)
			Expect(err).ShouldNot(HaveOccurred(), "unable to get ingresscontroller")

			originalCertName := ingresscontroller.Spec.DefaultCertificate.Name

			applicationIngressDefaultCertificateCanBeUpdated := func(ctx context.Context, name string) {
				ginkgo.GinkgoHelper()

				ginkgo.By("updating the PublishingStrategy's default ApplicationIngress certificate name")
				for i := range publishingstrategy.Spec.ApplicationIngress {
					if publishingstrategy.Spec.ApplicationIngress[i].Default {
						publishingstrategy.Spec.ApplicationIngress[i].Certificate.Name = name
					}
				}
				Expect(oc.Update(ctx, &publishingstrategy)).Should(Succeed(), "failed to update publishingstrategy")

				ginkgo.By(fmt.Sprintf("waiting for the default IngressController DefaultCertificate name to match %q", name))
				Eventually(ctx, func(ctx context.Context) (bool, error) {
					if err = oc.Get(ctx, "default", "openshift-ingress-operator", ingresscontroller); err != nil {
						return false, err
					}
					return ingresscontroller.Spec.DefaultCertificate.Name == name, nil
				})
			}

			applicationIngressDefaultCertificateCanBeUpdated(ctx, "osde2e")
			applicationIngressDefaultCertificateCanBeUpdated(ctx, originalCertName)
		})

		ginkgo.It("can be toggled from public to private", func(ctx context.Context) {
			ingressControllerVisiblityCanBeToggled := func(ctx context.Context, listening cloudingressv1alpha1.Listening) {
				ginkgo.GinkgoHelper()

				ginkgo.By("updating the PublishingStrategy's default ApplicationIngress listening state")
				for i := range publishingstrategy.Spec.ApplicationIngress {
					if publishingstrategy.Spec.ApplicationIngress[i].Default {
						publishingstrategy.Spec.ApplicationIngress[i].Listening = listening
					}
				}
				Expect(oc.Update(ctx, &publishingstrategy)).Should(Succeed(), "unable to update publishingstrategy with new dnsname")

				ginkgo.By("waiting for the change to be reflected in the openshift-ingress/router-default Service")
				Eventually(ctx, func(ctx context.Context) (bool, error) {
					service := new(corev1.Service)
					if err := oc.Get(ctx, "router-default", "openshift-ingress", service); err != nil {
						return false, err
					}
					_, ok := service.Annotations["service.beta.kubernetes.io/aws-load-balancer-internal"]
					if listening == cloudingressv1alpha1.External {
						return !ok, nil
					}
					return ok, nil
				})
			}

			ingressControllerVisiblityCanBeToggled(ctx, cloudingressv1alpha1.Internal)
			ingressControllerVisiblityCanBeToggled(ctx, cloudingressv1alpha1.External)
		})

		ginkgo.It("can have multiple ingresses", func(ctx context.Context) {
			ginkgo.By("creating a new ApplicationIngress")
			applicationingress := new(cloudingressv1alpha1.ApplicationIngress)
			for _, ai := range publishingstrategy.Spec.ApplicationIngress {
				if ai.Default {
					applicationingress = ai.DeepCopy()
				}
			}
			applicationingress.Default = false
			applicationingress.DNSName = "osde2e-" + applicationingress.DNSName
			publishingstrategy.Spec.ApplicationIngress = append(publishingstrategy.Spec.ApplicationIngress, *applicationingress)

			err := oc.Update(ctx, &publishingstrategy)
			Expect(err).ShouldNot(HaveOccurred(), "unable to update publishingstrategy with new ApplicationIngress")

			ingressControllerName := strings.Split(applicationingress.DNSName, ".")[0]

			Eventually(ctx, func(ctx context.Context) (bool, error) {
				ingresscontroller := new(operatorv1.IngressController)
				if err := oc.Get(ctx, ingressControllerName, "openshift-ingress-operator", ingresscontroller); err != nil {
					return false, err
				}
				return true, nil
			})
			EventuallyDeployment(ctx, oc, "router-"+ingressControllerName, "openshift-ingress")

			ginkgo.By("removing the additional ApplicationIngress from the PublishingStrategy")
			for i, ai := range publishingstrategy.Spec.ApplicationIngress {
				if strings.HasPrefix(ai.DNSName, "osde2e-") {
					publishingstrategy.Spec.ApplicationIngress = append(publishingstrategy.Spec.ApplicationIngress[:i], publishingstrategy.Spec.ApplicationIngress[i+1:]...)
				}
			}
			err = oc.Update(ctx, &publishingstrategy)
			Expect(err).ShouldNot(HaveOccurred(), "unable to update publishingstrategy with new ApplicationIngress")

			ginkgo.By("checking the IngressController is deleted")
			Eventually(ctx, func(ctx context.Context) (bool, error) {
				ingresscontroller := new(operatorv1.IngressController)
				err := oc.Get(ctx, ingressControllerName, "openshift-ingress-operator", ingresscontroller)
				if err != nil {
					// TODO: does this work?
					return apierrors.IsNotFound(err), err
				}
				return false, nil
			})
		})
	})
})
