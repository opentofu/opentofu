// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package oci

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

const delegationHeader = "opc-obo-token"

// Delegation is an environment-only modifier of the primary OCI identity.
// OCI provider legacy settings use lower-case attribute names after the OCI_
// prefix; upper-case aliases are accepted after all legacy sources.
func delegationSetting(name, fallback string) string {
	for _, key := range []string{"TF_VAR_" + name, "OCI_" + name, name, "OCI_" + strings.ToUpper(name)} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return fallback
}

func configureDelegation(client *objectstorage.ObjectStorageClient, provider common.ConfigurationProvider) error {
	enabled, err := strconv.ParseBool(delegationSetting("use_obo_token", "false"))
	if err != nil {
		return errors.New("OCI use_obo_token must be a boolean; check the delegation environment settings")
	}
	if !enabled {
		return nil
	}
	if client == nil || provider == nil {
		return errors.New("OCI delegation requires an initialized signing identity")
	}
	// Validate before tofu init succeeds, while continuing to reread the source
	// for every request so that file and environment token rotation is honored.
	if _, err := readDelegationToken(); err != nil {
		return err
	}
	previous := client.Interceptor
	client.Interceptor = func(request *http.Request) error {
		if previous != nil {
			if err := previous(request); err != nil {
				return err
			}
		}
		token, err := readDelegationToken()
		if err != nil {
			return err
		}
		request.Header.Set(delegationHeader, token)
		return nil
	}
	// PutObject copies the SDK signer to exclude body signing. That copy keeps
	// the generic signed headers, including this delegation header.
	headers := append(common.DefaultGenericHeaders(), delegationHeader)
	client.Signer = common.RequestSigner(provider, headers, common.DefaultBodyHeaders())
	return nil
}

func readDelegationToken() (string, error) {
	token := delegationSetting("obo_token", "")
	if filename := delegationSetting("obo_token_path", ""); filename != "" {
		material, err := os.ReadFile(expandAuthPath(filename))
		if err != nil {
			return "", errors.New("OCI delegation token file cannot be read; check obo_token_path and file permissions")
		}
		token = string(material)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("OCI delegation is enabled but its token is empty; set obo_token_path or obo_token")
	}
	if strings.IndexFunc(token, unicode.IsControl) >= 0 {
		return "", errors.New("OCI delegation token contains invalid header characters; replace it with a single-line token")
	}
	if recognizableJWT(token) {
		if err := validateExpiringToken(token, "OCI delegation"); err != nil {
			return "", err
		}
	}
	return token, nil
}

// Opaque delegation tokens were supported by the provider. Only recognize a
// JWT when its header identifies a signing algorithm; OCI validates authenticity.
func recognizableJWT(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	header, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[0], "="))
	if err != nil {
		return false
	}
	var fields struct {
		Algorithm string `json:"alg"`
	}
	return json.Unmarshal(header, &fields) == nil && fields.Algorithm != ""
}
