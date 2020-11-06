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

type DnsUpdateError struct {
	e string
}

func (e *DnsUpdateError) Error() string { return e.e }

func NewDNSUpdateError(reason string) error {
	return &DnsUpdateError{
		e: fmt.Sprintf("DNS Update Error %s", reason),
	}
}
