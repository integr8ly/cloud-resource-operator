package string

import (
	"math/rand"
	"time"
)

func stringFromBytes(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(65 + rand.Intn(90-65))
	}
	return string(b)
}

func RandomString(length int) string {
	rand.Seed(time.Now().UTC().UnixNano())
	return stringFromBytes(length)
}