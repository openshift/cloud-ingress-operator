package utils

import (
	"os"
	"testing"
)

func TestIsVersionHigher(t *testing.T) {

	tests := []struct {
		Name            string
		CompareExpected bool
		ErrorReason     string
		clusterversion  string
	}{
		{
			Name:            "Compare Cluster Version 4.10.44-rc",
			CompareExpected: true,
			ErrorReason:     "Wrong Comparision, it should be higher than 4.10 ",
			clusterversion:  "4.10.44-rc",
		},
		{
			Name:            "Compare Cluster Version 4.9.nightly.44-rc",
			CompareExpected: false,
			ErrorReason:     "Wrong Comparision, it should be lower than 4.10",
			clusterversion:  "4.9.nightly.44-rc",
		},
	}
	for _, test := range tests {

		defer os.Unsetenv("CLUSTER_VERSION")

		os.Setenv("CLUSTER_VERSION", test.clusterversion)

		CompareResult := IsVersionHigherThan("4.10")
		if CompareResult != test.CompareExpected {
			t.Errorf("Test [%v] return mismatch: [%v]. Expect  %t: Return %+v", test.Name, test.ErrorReason, test.CompareExpected, CompareResult)
		}

	}
}
