package errors

import (
	"fmt"
)

type LoadBalancerNotReadyError struct {
	e string
}

func (e *LoadBalancerNotReadyError) Error() string { return e.e }

func NewLoadBalancerNotReadyError() error {
	return &LoadBalancerNotReadyError{
		e: "Load balancer for service is not yet ready",
	}
}

type ForwardingRuleNotFoundError struct {
	e string
}

func (e *ForwardingRuleNotFoundError) Error() string { return e.e }

func ForwardingRuleNotFound(reason string) error {
	return &ForwardingRuleNotFoundError{
		e: "forwarding rule for svc not found in cloud provider. " + reason,
	}
}

type DnsUpdateError struct {
	e string
}

func (e *DnsUpdateError) Error() string { return e.e }

func NewDNSUpdateError(reason string) error {
	return &DnsUpdateError{
		e: fmt.Sprintf("DNS Update Error %s", reason),
	}
}
