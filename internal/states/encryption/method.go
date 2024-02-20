// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package encryption

type Method interface {
	Encrypt(payload []byte) ([]byte, error)
	Decrypt(payload []byte) ([]byte, error)
}

type passthrough struct{}

func (p passthrough) Encrypt(payload []byte) ([]byte, error) {
	return payload, nil
}
func (p passthrough) Decrypt(payload []byte) ([]byte, error) {
	// TODO check that payload is valid json, see sniffJSONStateVersion as an example
	return payload, nil
}

func Passthrough() Method {
	return passthrough{}
}
