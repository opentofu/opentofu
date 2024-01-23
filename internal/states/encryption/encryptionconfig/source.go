package encryptionconfig

type Source string

const (
	// SourceCode indicates that the configuration has been obtained by parsing the HCL/JSON/etc source.
	SourceCode Source = "code"
	// SourceEnv indicates that a configuration has been obtained by using ConfigurationFromEnv from the operating
	// system environment.
	SourceEnv Source = "env"
)

func (s Source) IsValid() bool {
	return s == SourceCode || s == SourceEnv
}
