package aws

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestNewConfigForStaticCreds(t *testing.T) {
	cases := []struct {
		key          string
		secret       string
		sharedConfig string
	}{{
		key:    `asdf`,
		secret: `asdf1234`,
		sharedConfig: `[default]
aws_access_key_id = asdf
aws_secret_access_key = asdf1234
`,
	}}

	for _, test := range cases {
		t.Run("", func(t *testing.T) {
			sharedConfig := newConfigForStaticCreds(test.key, test.secret)
			assert.Equal(t, string(sharedConfig), test.sharedConfig)
		})
	}
}

func TestSharedCredentialsFileFromSecret(t *testing.T) {
	cases := []struct {
		data         map[string]string
		sharedConfig string
		err          string
	}{{
		data: map[string]string{
			"aws_access_key_id":     "asdf",
			"aws_secret_access_key": "asdf1234",
		},
		sharedConfig: `[default]
aws_access_key_id = asdf
aws_secret_access_key = asdf1234
`,
	}, {
		data: map[string]string{
			"credentials": `[default]
assume_role = role_for_cloud_ingress_operator
web_identity_token = /path/to/sa/token
`,
		},

		sharedConfig: `[default]
assume_role = role_for_cloud_ingress_operator
web_identity_token = /path/to/sa/token
`,
	}, {
		data: map[string]string{
			"wrong_format_cred_file": "random_value",
		},
		// err should match the defaut case for SharedCredentialsFileFromSecret() func
		// and return the exact same error message of `invalid secret for aws credentials`
		err: "invalid secret for aws credentials",
	}, {
		data: map[string]string{
			"aws_access_key_id":     "asdf",
			"aws_secret_access_key": "asdf1234",
			"credentials": `[default]
aws_access_key_id = asdf
aws_secret_access_key = asdf1234
`,
		},
		sharedConfig: `[default]
aws_access_key_id = asdf
aws_secret_access_key = asdf1234
`,
	}}

	for _, test := range cases {
		t.Run("", func(t *testing.T) {
			secret := &corev1.Secret{
				Data: map[string][]byte{},
			}

			for k, v := range test.data {
				secret.Data[k] = []byte(v)
			}

			credPath, err := SharedCredentialsFileFromSecret(secret)
			if credPath != "" {
				defer os.Remove(credPath)
			}

			if test.err == "" {
				assert.NoError(t, err)
				data, err := os.ReadFile(credPath)
				t.Log(data)
				assert.NoError(t, err)
				assert.Equal(t, string(data), test.sharedConfig)
			} else {
				assert.Regexp(t, test.err, err)
			}
		})
	}
}
