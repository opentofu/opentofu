package aesgcm

import (
	"bytes"
	"errors"
	"testing"

	"github.com/opentofu/opentofu/internal/encryption/keyprovider"

	"github.com/opentofu/opentofu/internal/encryption/method"
)

func TestConfig_Build(t *testing.T) {
	descriptor := New()
	var testCases = []struct {
		name      string
		config    *Config
		errorType any
		expected  aesgcm
	}{
		{
			name: "key-32-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				nil,
			},
		},
		{
			name: "key-24-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileefe"),
				[]byte("bohwu9zoo7Zool5olaileefd"),
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5olaileefe"),
				[]byte("bohwu9zoo7Zool5olaileefd"),
				nil,
			},
		},
		{
			name: "key-16-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5e"),
				[]byte("bohwu9zoo7Zool5d"),
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5e"),
				[]byte("bohwu9zoo7Zool5d"),
				nil,
			},
		},
		{
			name:      "no-key",
			config:    descriptor.TypedConfig(),
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "encryption-key-15-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Ze15"),
				[]byte("bohwu9zoo7Zod16"),
				nil,
			}),
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "decryption-key-15-bytes",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zooe16"),
				[]byte("bohwu9zoo7Zod15"),
				nil,
			}),
			errorType: &method.ErrInvalidConfiguration{},
		},
		{
			name: "decryption-key-fallback",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zooe16"),
				nil,
				nil,
			}),
			errorType: nil,
			expected: aesgcm{
				[]byte("bohwu9zoo7Zooe16"),
				[]byte("bohwu9zoo7Zooe16"),
				nil,
			},
		},
		{
			name: "aad",
			config: descriptor.TypedConfig().WithKeys(keyprovider.Output{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				nil,
			}).WithAAD([]byte("foobar")),
			expected: aesgcm{
				[]byte("bohwu9zoo7Zool5olaileef1eibeathe"),
				[]byte("bohwu9zoo7Zool5olaileef1eibeathd"),
				[]byte("foobar"),
			},
			errorType: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			built, err := tc.config.Build()
			if tc.errorType == nil {
				if err != nil {
					t.Fatalf("Unexpected error returned: %v", err)
				}

				built := built.(*aesgcm)

				if !bytes.Equal(tc.expected.encryptionKey, built.encryptionKey) {
					t.Fatalf("Incorrect encryption key built: %v != %v", tc.expected.encryptionKey, built.encryptionKey)
				}
				if !bytes.Equal(tc.expected.decryptionKey, built.decryptionKey) {
					t.Fatalf("Incorrect decryption key built: %v != %v", tc.expected.decryptionKey, built.decryptionKey)
				}
				if !bytes.Equal(tc.expected.aad, built.aad) {
					t.Fatalf("Incorrect aad built: %v != %v", tc.expected.aad, built.aad)
				}

			} else if tc.errorType != nil {
				if err == nil {
					t.Fatal("Expected error, none received")
				}
				if !errors.As(err, &tc.errorType) {
					t.Fatalf("Incorrect error type received: %T", err)
				}
				t.Logf("Correct error of type %T received: %v", err, err)
			}

		})
	}
}
