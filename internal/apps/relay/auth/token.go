package auth

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const tokenChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// GenerateToken generates a random token of the specified length.
func GenerateToken(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(tokenChars))))
		if err != nil {
			return "", fmt.Errorf("generate random number: %w", err)
		}
		result[i] = tokenChars[num.Int64()]
	}
	return string(result), nil
}

// GenerateOwnerToken generates a random owner token.
func GenerateOwnerToken() (string, error) {
	return GenerateToken(32)
}
