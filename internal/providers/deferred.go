package providers

type Deferred struct {
	Reason DeferredReason
}

type DeferredReason int32

const (
	// DeferredReasonUnknown is the default value, and should not be used.
	DeferredReasonUnknown DeferredReason = 0
	// DeferredReasonResourceConfigUnknown is used when the config is partially unknown and the real
	// values need to be known before the change can be planned.
	DeferredReasonResourceConfigUnknown DeferredReason = 1
	// DeferredReasonProviderConfigUnknown is used when parts of the provider configuration
	// are unknown, e.g. the provider configuration is only known after the apply is done.
	DeferredReasonProviderConfigUnknown DeferredReason = 2
	// DeferredReasonAbsentPrereq is used when a hard dependency has not been satisfied.
	DeferredReasonAbsentPrereq DeferredReason = 3
)

func (o DeferredReason) String() string {
	switch o {
	case DeferredReasonProviderConfigUnknown:
		return "Provider config unknown"
	case DeferredReasonResourceConfigUnknown:
		return "Resource config unknown"
	case DeferredReasonAbsentPrereq:
		return "Absent prerequisites"
	default: // DeferredReasonUnknown
		return "Unknown deferred reason"
	}
}
