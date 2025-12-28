package app

import (
	"math/rand"
)

func randomInt(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}
