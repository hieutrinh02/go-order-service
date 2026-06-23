package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func HashRequestBody(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}
