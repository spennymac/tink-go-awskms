// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
////////////////////////////////////////////////////////////////////////////////

package awskms_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	// Placeholder for internal flag import.
	// context is used to cancel outstanding requests
	kmsv2 "github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/tink-crypto/tink-go/v2/aead"
	"github.com/tink-crypto/tink-go/v2/core/registry"
	"github.com/tink-crypto/tink-go/v2/tink"

	"github.com/tink-crypto/tink-go-awskms/v2/integration/awskms"
)

const (
	keyPrefix   = "aws-kms://arn:aws:kms:us-east-2:235739564943:"
	keyAliasURI = "aws-kms://arn:aws:kms:us-east-2:235739564943:alias/unit-and-integration-testing"
	keyURI      = "aws-kms://arn:aws:kms:us-east-2:235739564943:key/3ee50705-5a82-4f5b-9753-05c4f473922f"
	keyURI2     = "aws-kms://arn:aws:kms:us-east-2:235739564943:key/b3ca2efd-a8fb-47f2-b541-7e20f8c5cd11"
)

var (
	credCSVFile = "testdata/aws/credentials.csv"
	credINIFile = "testdata/aws/credentials.ini"
)

// Placeholder for internal initialization.

func getTestFilePath(filename string) (string, error) {
	srcDir, ok := os.LookupEnv("TEST_SRCDIR")
	if !ok {
		return "", errors.New("TEST_SRCDIR not set")
	}
	workspaceDir, ok := os.LookupEnv("TEST_WORKSPACE")
	if !ok {
		return "", errors.New("TEST_WORKSPACE not set")
	}
	return filepath.Join(srcDir, workspaceDir, filename), nil
}

func TestNewClientWithCredentialsGetAEADEncryptDecrypt(t *testing.T) {
	credFilePath, err := getTestFilePath(credCSVFile)
	if err != nil {
		t.Skip(err)
	}
	client, err := awskms.NewClientWithOptions(keyURI, awskms.WithCredentialPath(credFilePath))
	if err != nil {
		t.Fatalf("error setting up AWS client: %v", err)
	}
	a, err := client.GetAEAD(keyURI)
	if err != nil {
		t.Fatalf("client.GetAEAD(keyURI) err = %v, want nil", err)
	}
	plaintext := []byte("plaintext")
	associatedData := []byte("associatedData")
	ciphertext, err := a.Encrypt(plaintext, associatedData)
	if err != nil {
		t.Fatalf("a.Encrypt(plaintext, associatedData) err = %v, want nil", err)
	}
	gotPlaintext, err := a.Decrypt(ciphertext, associatedData)
	if err != nil {
		t.Fatalf("a.Decrypt(ciphertext, associatedData) err = %v, want nil", err)
	}
	if !bytes.Equal(gotPlaintext, plaintext) {
		t.Errorf("a.Decrypt() = %q, want %q", gotPlaintext, plaintext)
	}

	invalidAssociatedData := []byte("invalidAssociatedData")
	_, err = a.Decrypt(ciphertext, invalidAssociatedData)
	if err == nil {
		t.Error("a.Decrypt(ciphertext, invalidAssociatedData) err = nil, want error")
	}
}

func TestEmptyAssociatedDataEncryptDecrypt(t *testing.T) {
	credFilePath, err := getTestFilePath(credCSVFile)
	if err != nil {
		t.Skip(err)
	}
	client, err := awskms.NewClientWithOptions(keyURI, awskms.WithCredentialPath(credFilePath))
	if err != nil {
		t.Fatalf("error setting up AWS client: %v", err)
	}
	a, err := client.GetAEAD(keyURI)
	if err != nil {
		t.Fatalf("client.GetAEAD(keyURI) err = %v, want nil", err)
	}
	plaintext := []byte("plaintext")
	emptyAssociatedData := []byte{}
	ciphertext, err := a.Encrypt(plaintext, emptyAssociatedData)
	if err != nil {
		t.Fatalf("a.Encrypt(plaintext, emptyAssociatedData) err = %v, want nil", err)
	}
	gotPlaintext, err := a.Decrypt(ciphertext, emptyAssociatedData)
	if err != nil {
		t.Fatalf("a.Decrypt(ciphertext, associatedData) err = %v, want nil", err)
	}
	if !bytes.Equal(gotPlaintext, plaintext) {
		t.Errorf("a.Decrypt() = %q, want %q", gotPlaintext, plaintext)
	}

	gotPlaintext2, err := a.Decrypt(ciphertext, nil)
	if err != nil {
		t.Fatalf("a.Decrypt(ciphertext, nil) err = %v, want nil", err)
	}
	if !bytes.Equal(gotPlaintext2, plaintext) {
		t.Errorf("a.Decrypt() = %q, want %q", gotPlaintext, plaintext)
	}
}

func TestKeyCommitment(t *testing.T) {
	credFilePath, err := getTestFilePath(credCSVFile)
	if err != nil {
		t.Skip(err)
	}
	client, err := awskms.NewClientWithOptions(keyPrefix, awskms.WithCredentialPath(credFilePath))
	if err != nil {
		t.Fatalf("error setting up AWS client: %v", err)
	}

	// Create AEAD primitives for two keys.
	keys := []string{keyURI, keyURI2}
	aeads := make([]tink.AEAD, 0, len(keys))
	for _, k := range keys {
		a, err := client.GetAEAD(k)
		if err != nil {
			t.Fatalf("client.GetAEAD(keyURI) err = %v, want nil", err)
		}
		aeads = append(aeads, a)
	}

	// Create a ciphertext using the first primitive.
	plaintext := []byte("plaintext")
	associatedData := []byte("associated data")
	ciphertext, err := aeads[0].Encrypt(plaintext, associatedData)
	if err != nil {
		t.Fatalf("aeads[0].Encrypt(plaintext, associatedData) err = %v, want nil", err)
	}
	gotPlaintext, err := aeads[0].Decrypt(ciphertext, associatedData)
	if err != nil {
		t.Fatalf("aeads[0].Decrypt(ciphertext, associatedData) err = %v, want nil", err)
	}
	if !bytes.Equal(gotPlaintext, plaintext) {
		t.Errorf("aeads[0].Decrypt() = %q, want %q", gotPlaintext, plaintext)
	}

	// Attempt to decrypt using the other primitive.
	_, err = aeads[1].Decrypt(ciphertext, associatedData)
	if err == nil {
		t.Fatalf("aeads[1].Decrypt(ciphertext, associatedData) err = nil, want non-nil")
	}
}

func TestKMSEnvelopeAEADEncryptAndDecrypt(t *testing.T) {
	tests := []struct {
		name           string
		credsFilePaths string
		clientFn       func(*testing.T) registry.KMSClient
	}{
		{
			name: "v1 encrypt/decrypt with CSV credentials",
			clientFn: func(t *testing.T) registry.KMSClient {
				credFilePath, err := getTestFilePath(credCSVFile)
				if err != nil {
					t.Skip(err)
				}
				client, err := awskms.NewClientWithOptions(keyURI, awskms.WithCredentialPath(credFilePath))
				if err != nil {
					t.Fatalf("awskms.NewClientWithOptions() err = %q, want nil", err)
				}
				return client
			},
		},
		{
			name: "v1 encrypt/decrypt with INI credentials",
			clientFn: func(t *testing.T) registry.KMSClient {
				credFilePath, err := getTestFilePath(credINIFile)
				if err != nil {
					t.Skip(err)
				}
				client, err := awskms.NewClientWithOptions(keyURI, awskms.WithCredentialPath(credFilePath))
				if err != nil {
					t.Fatalf("awskms.NewClientWithOptions() err = %q, want nil", err)
				}
				return client
			},
		},
		{
			name: "v1 encrypt/decrypt with INI credentials",
			clientFn: func(t *testing.T) registry.KMSClient {
				credFilePath, err := getTestFilePath(credINIFile)
				if err != nil {
					t.Skip(err)
				}
				client, err := awskms.NewClientWithOptions(keyURI, awskms.WithCredentialPath(credFilePath))
				if err != nil {
					t.Fatalf("awskms.NewClientWithOptions() err = %q, want nil", err)
				}
				return client
			},
		},
		{
			name: "v2 encrypt/decrypt with INI credentials",
			clientFn: func(t *testing.T) registry.KMSClient {
				credFilePath, err := getTestFilePath(credINIFile)
				if err != nil {
					t.Skip(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
				defer cancel()

				cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedCredentialsFiles([]string{credFilePath}))
				if err != nil {
					t.Errorf("loading AWS config: %v", err)
				}

				kmsClient, err := kmsv2.NewFromConfig(cfg), nil
				if err != nil {
					t.Errorf("creating kms client: %v", err)
				}

				client, err := awskms.NewClientWithOptions(keyURI, awskms.WithV2KMS(kmsClient))
				if err != nil {
					t.Fatalf("awskms.NewClientWithOptions() err = %q, want nil", err)
				}
				return client
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := test.clientFn(t)
			kekAEAD, err := client.GetAEAD(keyURI)
			if err != nil {
				t.Fatalf("client.GetAEAD(keyURI) err = %q, want nil", err)
			}

			dekTemplate := aead.AES128CTRHMACSHA256KeyTemplate()
			a := aead.NewKMSEnvelopeAEAD2(dekTemplate, kekAEAD)
			if err != nil {
				t.Fatalf("aead.NewKMSEnvelopeAEAD2(dekTemplate, kekAEAD) err = %q, want nil", err)
			}
			plaintext := []byte("plaintext")
			for _, associatedData := range [][]byte{nil, []byte("associated data")} {
				ciphertext, err := a.Encrypt(plaintext, associatedData)
				if err != nil {
					t.Fatalf("a.Encrypt(plaintext, associatedData) err = %q, want nil", err)
				}
				gotPlaintext, err := a.Decrypt(ciphertext, associatedData)
				if err != nil {
					t.Fatalf("a.Decrypt(ciphertext, associatedData) err = %q, want nil", err)
				}
				if !bytes.Equal(gotPlaintext, plaintext) {
					t.Errorf("a.Decrypt() = %q, want %q", gotPlaintext, plaintext)
				}
			}
		})
	}
}
