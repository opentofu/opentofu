package encryption

// Encryption contains the methods for obtaining a State or Plan correctly configured for a specific
// purpose. If no encryption configuration is present, it returns a passthru method that doesn't do anything.
type Encryption interface {
	StateFile() State
	PlanFile() Plan
	Backend() State
	// RemoteState returns a State suitable for a remote state data source. Note: this function panics
	// if the path of the remote state data source is invalid, but does not panic if it is incorrect.
	RemoteState(string) State
}

func New(reg Registry, config Config) Encryption {
	return nil
}
