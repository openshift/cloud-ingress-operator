package utils

import (
	"testing"
)

func TestHasUserManagedIngressFeature(t *testing.T) {
	type args struct {
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "It does not have the feature enabled if the label does not exist on the cluster",
			args: args{
				labels: nil,
			},
			want: false,
		},
		{
			name: "It does not have the feature enabled if the label exists and is true",
			args: args{
				labels: map[string]string{
					ClusterLegacyIngressLabel: "true",
				},
			},
			want: false,
		},
		{
			name: "It has the feature enabled if the label exists and is false",
			args: args{
				labels: map[string]string{
					ClusterLegacyIngressLabel: "false",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasUserManagedIngressFeature(tt.args.labels); got != tt.want {
				t.Errorf("HasUserManagedIngressFeature() = %v, want %v", got, tt.want)
			}
		})
	}
}
