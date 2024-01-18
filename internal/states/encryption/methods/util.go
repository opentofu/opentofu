package methods

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/states/encryption/encryptionconfig"
	"github.com/opentofu/opentofu/internal/states/encryption/encryptionflow"
)

// decryptAESGCM is the low level primitive used to decrypt a single payload section.
//
// salt must have the same value that was used for encryption.
func decryptAESGCM(payload []byte, key []byte, salt []byte) ([]byte, error) {
	if len(salt) < 16 {
		return nil, errors.New("salt not provided or too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("could not create block cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("could not create GCM: %w", err)
	}

	if len(payload) < aesgcm.NonceSize() {
		return nil, fmt.Errorf("encrypted payload too short, not even enough for the nonce")
	}

	nonce := payload[0:aesgcm.NonceSize()]
	ciphertextOffset := len(nonce)
	ciphertext := payload[ciphertextOffset:]

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, salt)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt: %w", err)
	}

	return plaintext, nil
}

// encryptAESGCM is the low level primitive used to encrypt a single plaintext section.
//
// salt must be stored so the same value can be provided again for decryption.
// It is also used for hashing the payload to detect encryption errors.
func encryptAESGCM(plaintext []byte, key []byte, salt []byte, allowEmpty bool) ([]byte, error) {
	if len(plaintext) == 0 && !allowEmpty {
		return nil, errors.New("plaintext is empty")
	}

	if len(salt) < 16 {
		return nil, errors.New("salt not provided or too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("could not create block cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("could not create GCM: %w", err)
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("could not generate nonce: %w", err)
	}

	// salt is added as additional data so that it's authenticated
	ciphertext := aesgcm.Seal(nil, nonce, plaintext, salt)

	var dataToStore bytes.Buffer
	dataToStore.Write(nonce)
	dataToStore.Write(ciphertext)

	return dataToStore.Bytes(), nil
}

func toMarkedBase64(encrypted []byte) string {
	return "ENC[" + base64.StdEncoding.EncodeToString(encrypted) + "]"
}

func fromMarkedBase64(markedBase64Raw any) ([]byte, error) {
	markedBase64, ok := markedBase64Raw.(string)
	if !ok {
		return nil, errors.New("payload was not of type string - not produced with this method")
	}
	if !strings.HasPrefix(markedBase64, "ENC[") {
		return nil, errors.New("encrypted string did not have prefix - not produced with this method")
	}
	if !strings.HasSuffix(markedBase64, "]") {
		return nil, errors.New("encrypted string did not have suffix - probably truncated")
	}
	unmarked := strings.TrimPrefix(strings.TrimSuffix(markedBase64, "]"), "ENC[")

	raw, err := base64.StdEncoding.DecodeString(unmarked)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func provideAndRecordMethodSalt(info *encryptionflow.EncryptionInfo, name encryptionconfig.MethodName) ([]byte, error) {
	if info.Method.Config == nil {
		// we are encrypting - generate a new salt and put it in info

		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			return nil, fmt.Errorf("could not generate salt: %w", err)
		}

		hexSalt := hex.EncodeToString(salt)
		info.Method.Config = make(map[string]string)
		info.Method.Config["salt"] = hexSalt

		return salt, nil
	} else {
		// we are decrypting - read salt from info

		hexSalt, ok := info.Method.Config["salt"]
		if !ok {
			return nil, fmt.Errorf("state or plan corrupt for method %s - missing salt", name)
		}
		salt, err := hex.DecodeString(hexSalt)
		if err != nil {
			return nil, fmt.Errorf("state or plan corrupt for method %s - failed to decode salt", name)
		}
		if len(salt) < 16 {
			return nil, fmt.Errorf("state or plan corrupt for method %s - failed to decode salt", name)
		}
		return salt, nil
	}
}
