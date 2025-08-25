package main

import (
	"os"
	"reflect"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/cache"
)

func TestGetWatchNamespaces(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		expectError bool
		expectKeys  []string
	}{
		{
			name:        "empty env returns error",
			envValue:    "",
			expectError: true,
			expectKeys:  nil,
		},
		{
			name:        "single namespace",
			envValue:    "ns1",
			expectError: false,
			expectKeys:  []string{"ns1"},
		},
		{
			name:        "multiple namespaces comma separated",
			envValue:    "ns1,ns2,ns3",
			expectError: false,
			expectKeys:  []string{"ns1", "ns2", "ns3"},
		},
		{
			name:        "trims whitespace around namespaces",
			envValue:    " ns1 , ns2 ",
			expectError: false,
			expectKeys:  []string{"ns1", "ns2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(watchNamespaceEnvVar, tt.envValue)
			defer os.Unsetenv(watchNamespaceEnvVar)

			nsMap, err := getWatchNamespaces()
			if tt.expectError && err == nil {
				t.Fatalf("expected error, got none")
			}
			if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.expectError {
				for _, key := range tt.expectKeys {
					if _, ok := nsMap[key]; !ok {
						t.Errorf("expected key %s in map, but not found", key)
					}
				}
				// Check size matches
				if len(nsMap) != len(tt.expectKeys) {
					t.Errorf("expected %d namespaces, got %d", len(tt.expectKeys), len(nsMap))
				}
				// Ensure all values are of type cache.Config
				for k, v := range nsMap {
					if !reflect.DeepEqual(v, cache.Config{}) {
						t.Errorf("expected empty cache.Config for key %s, got %#v", k, v)
					}
				}
			}
		})
	}
}
