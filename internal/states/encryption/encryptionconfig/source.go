package encryptionconfig

type Source string

const (
	// SourceHCL indicates that the configuration has been obtained by parsing the HCL source.
	SourceHCL Source = "hcl"
	// SourceEnv indicates that a configuration has been obtained by using ConfigurationFromEnv from the operating
	// system environment.
	SourceEnv Source = "env"
)

func (s Source) IsValid() bool {
	return s == SourceHCL || s == SourceEnv
}
