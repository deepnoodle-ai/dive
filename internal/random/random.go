package random

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Integer returns a random integer as a string
func Integer() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%d", n)
}
