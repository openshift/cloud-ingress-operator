package aws

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuild(t *testing.T) {
	secret := &corev1.Secret{
		Data: make(map[string][]byte),
	}
	secret.Data["aws_access_key_id"] = []byte("fake")
	secret.Data["aws_secret_access_key"] = []byte("fake")
	region := "sut"

	_, err := build(secret, region)
	if err != nil {
		t.Errorf("cli couldn't initialized: %w", err)
	}
}

func TestConfigure(t *testing.T) {
	_, err := configure("XXX-SS", "XXX-TT", "eu-west-1")
	if err != nil {
		t.Errorf("cli couldn't configured: %w", err)
	}
}
