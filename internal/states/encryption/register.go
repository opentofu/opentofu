package encryption

import (
	"github.com/opentofu/opentofu/internal/states/encryption/keyproviders"
	"github.com/opentofu/opentofu/internal/states/encryption/methods"
)

func init() {
	keyproviders.RegisterDirectKeyProvider()
	keyproviders.RegisterPassphraseKeyProvider()

	methods.RegisterFullMethod()
	methods.RegisterPartialMethod()
}
