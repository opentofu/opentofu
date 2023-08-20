package cliconfig

import "os"

func getNewOrLegacyPath(newPath string, legacyPath string) (string, error) {
	// If the legacy directory exists, but the new directory does not, then use the legacy directory, for backwards compatibility reasons.
	// Otherwise, use the new directory.
	if _, err := os.Stat(legacyPath); err == nil {
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return legacyPath, nil
		}
	}

	return newPath, nil
}
