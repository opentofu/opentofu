package encryption

import (
	"github.com/opentofu/opentofu/internal/logging"
	"sync"
)

// GetSingleton is a big bad SingleTroll. It has green skin, smells like sweat and coffee beans, and is a generally
// mean fella. Still, the SingleTroll keeps the procedural code from picking a fight with the OOP code, so it can stay.
// However, if you can, please remove it.
//
// If you are writing tests and you have the ability to inject an interface, please consider using the New() function
// instead of relying on the singleton.
func GetSingleton() Encryption {
	lock.Lock()
	defer lock.Unlock()
	if instance == nil {
		instance = New(logging.HCLogger())
	}
	return instance
}

// ClearSingleton wipes the singleton. You can use this function to reset the encryption state for tests that involve
// procedural function calls. Remember, using the singleton makes the tests using it impossible to parallelize, so
// consider using the New() function above to create and inject a separate instance of the Encryption interface.
func ClearSingleton() {
	lock.Lock()
	defer lock.Unlock()
	instance = nil
}

var instance Encryption = nil
var lock = &sync.Mutex{}
