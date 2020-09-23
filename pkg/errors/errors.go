package errors

import (
	"fmt"
)

type LoadBalancerNotFoundError struct {
	e string
}

func (e *LoadBalancerNotFoundError) Error() string { return e.e }

func NewLoadBalancerNotFoundError(lb string) error {
	return &LoadBalancerNotFoundError{
		e: fmt.Sprintf("Could not find LoadBalancer %s", lb),
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
