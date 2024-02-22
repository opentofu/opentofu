package aesgcm

import "testing"

type testCase struct {
	aes   *aesgcm
	error bool
}

func TestInternalErrorHandling(t *testing.T) {
	testCases := map[string]testCase{
		"ok": {
			&aesgcm{
				key:       []byte("aeshi1quahb2Rua0ooquaiwahbonedoh"),
				tagSize:   defaultTagSize,
				nonceSize: defaultNonceSize,
			},
			false,
		},
		"no-key": {
			&aesgcm{},
			true,
		},
		"empty-nonce": {
			&aesgcm{
				key:       []byte("aeshi1quahb2Rua0ooquaiwahbonedoh"),
				tagSize:   defaultTagSize,
				nonceSize: 0,
			},
			true,
		},
		"too-short-tag": {
			&aesgcm{
				key:       []byte("aeshi1quahb2Rua0ooquaiwahbonedoh"),
				tagSize:   11,
				nonceSize: defaultNonceSize,
			},
			true,
		},
		"too-long-tag": {
			&aesgcm{
				key:       []byte("aeshi1quahb2Rua0ooquaiwahbonedoh"),
				tagSize:   17,
				nonceSize: defaultNonceSize,
			},
			true,
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			encrypted, err := tc.aes.Encrypt([]byte("Hello world!"))
			if tc.error && err == nil {
				t.Fatalf("Expected error, none returned.")
			} else if !tc.error && err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !tc.error {
				decrypted, err := tc.aes.Decrypt(encrypted)
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if string(decrypted) != "Hello world!" {
					t.Fatalf("Incorrect decrypted string: %s", decrypted)
				}
			}
		})
	}
}
