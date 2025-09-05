package auth

import (
	"fmt"
	"os"
)

func consolidateFileAndValue(value, fileName, fieldName string) (string, error) {
	if fileName == "" {
		return value, nil
	}

	b, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("error reading %s file: %w", fieldName, err)
	}
	fileValue := string(b)
	if value != "" && value != fileValue {
		return "", fmt.Errorf("%s provided directly and through file do not match; either make them the same value or only provide one", fieldName)
	}
	return fileValue, nil
}
