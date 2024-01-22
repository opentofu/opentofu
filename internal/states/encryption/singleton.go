package encryption

import (
	"sync"

	"github.com/opentofu/opentofu/internal/logging"
)

// GetSingleton is a big bad SingleTroll. It has green skin, smells like sweat and coffee beans, and is a generally
// mean fella. Still, the SingleTroll keeps the procedural code from picking a fight with the OOP code, so it can stay.
// However, if you can, please remove it.
//
// On a more serious note, a large portion of the OpenTofu codebase is still procedural, which means there is no way to
// properly inject the Encryption struct and carry the information it holds across subsystem boundaries. In most cases
// you should use this function to get a globally scoped copy of the Encryption object. However, for tests you should
// use the New() function and hopefully, some time in the future, we can get rid of the singleton entirely.
//
// If you are writing tests and you have the ability to inject an interface, please consider using the New() function
// instead of relying on the singleton. If you must rely on this singleton, make sure that tests are not running in
// parallel and that you call ClearSingleton() in the cleanup function.
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
// consider using the New() function to create and inject a separate instance of the Encryption interface.
func ClearSingleton() {
	lock.Lock()
	defer lock.Unlock()
	instance = nil
}

var instance Encryption = nil
var lock = &sync.Mutex{}
