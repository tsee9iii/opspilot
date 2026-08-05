package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type HMACHasher struct {
	key []byte
}

func NewHMACHasher(secret string) *HMACHasher {
	return &HMACHasher{key: []byte(secret)}
}

func (h *HMACHasher) Hash(token string) string {
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
