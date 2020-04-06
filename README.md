# cloud-ingress-operator

Cloud-ingress-operator is designed to assist in toggling OpenShift Dedicated 4.x clusters "private" and "public" through the use of custom Kubernetes resources.

There are two pieces of the cluster which can be toggled in this way: The default API server (`api.<cluster-domain>`) and the ingresses (the default being `*.apps.<cluster-domain>`, and up to one additional, named `*.apps2.<cluster-domain>`). These may be handled independently of one another in the manner described below.

To ensure that [Hive](https://github.com/openshift/hive) and SRE have continued access to manage the cluster on behalf of the customer, an additional API endpoint is created, named by default `rh-api.<cluster-domain>`. This endpoint may also be referred to as the "admin endpoint," or "admin API endpoint."

The operator does not manage any kind of VPC peering or VPN connectivity into the clustered environment.

## Controlling the Operator

As mentioned above, the operator is controlled through custom Kubernetes resources, `APIScheme` and `PublishingStrategy`. They are documented below:

### APIScheme Custom Resource

The APIScheme resource instructs the operator to create the admin API endpoint. The specification of the resource is explained below:

```yaml
spec:
  managementAPIServerIngress:
    enabled: true
    dnsName: rh-api
    allowedCIDRBlocks:
      - "0.0.0.0/0"
```

In this example, the endpoint will be called `rh-api` and the full name `rh-api.<cluster-domain>`. Furthermore, there will be a single entry in the security group associated with the cloud load balancer that allows `0.0.0.0/0` (everything).

### Toggling Privacy

Toggling privacy is done with the `PublishingStrategy` custom resource.

```yaml
spec:
  defaultAPIServerIngress:
    listening: external
  applicationIngress:
    - listening: external
      default: true
      dnsName: "*.apps"
      certificate:
        secretRef:
          name: foo
      routeSelector:
        labelSelector:
          matchLabels:
            foo: bar
```

In this example, the default API server (`api.<cluster-domain>`) is configured to be externally available ("public") and the default ingress (named `*.apps.<cluster-domain>`, and identified by `default: true`) is configured to also be externally available ("public"). Additionally, the default ingress is configured to use a TLS certificate named `foo`, in the `openshift-ingress` namespace. Finally, the ingress uses the route selector as specified.

Note: the `namespace` attribute for `secretRef` is not currently used; certificates must be within the `openshift-ingress` namespace.

It is possible to add additional applicationIngresses, however at this time, OSD supports the default plus an additional.

# Development

The operator is built with [operator-sdk](https://github.com/operator-framework/operator-sdk). There is a tight dependency on the AWS cluster provider but the dependency is pinned to the [OpenShift fork](https://github.com/openshift/cluster-api-provider-aws) for access to v1beta1 API features.
