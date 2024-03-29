package aesgcm

import (
	"fmt"
)

func Example_handlePanic() {
	_, err := handlePanic(func() ([]byte, error) {
		panic("Hello world!")
	})
	fmt.Printf("%v", err)
	// Output: Hello world!
}
