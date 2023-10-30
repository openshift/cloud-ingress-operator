package aws

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
)

// SharedCredentialsFileFromSecret returns a path to the shared creds file created using provided secret
// configure the aws session using file to use credentials eg
// sharedCredentialsFile, err := SharedCredentialsFileFromSecret(secret)
//
//	if err != nil {
//		// handle error
//	}
//
//	options := session.Options{
//		SharedConfigState: session.SharedConfigEnable,
//		SharedConfigFiles: []string{sharedCredentialsFile},
//	}
//
// sess := session.Must(session.NewSessionWithOptions(options))
func SharedCredentialsFileFromSecret(secret *corev1.Secret) (string, error) {
	var data []byte
	switch {
	case len(secret.Data["credentials"]) > 0:
		data = secret.Data["credentials"]
	case len(secret.Data["aws_access_key_id"]) > 0 && len(secret.Data["aws_secret_access_key"]) > 0:
		data = newConfigForStaticCreds(
			string(secret.Data["aws_access_key_id"]),
			string(secret.Data["aws_secret_access_key"]),
		)

	default:
		return "", errors.New("invalid secret for aws credentials")

	}

	f, err := os.CreateTemp("", "aws-shared-credentials")
	if err != nil {
		return "", fmt.Errorf("failed to create file for shared credentials: %v", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return "", fmt.Errorf("failed to write credentials to %s: %v", f.Name(), err)
	}

	return f.Name(), nil
}

func newConfigForStaticCreds(accessKey string, accessSecret string) []byte {
	buf := &bytes.Buffer{}
	fmt.Fprint(buf, "[default]\n")
	fmt.Fprintf(buf, "aws_access_key_id = %s\n", accessKey)
	fmt.Fprintf(buf, "aws_secret_access_key = %s\n", accessSecret)
	return buf.Bytes()
}
