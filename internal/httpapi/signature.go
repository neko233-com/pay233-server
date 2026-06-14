package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

const (
	headerSignature = "X-Pay233-Signature"
	headerTimestamp = "X-Pay233-Timestamp"
)

func signPayload(secret string, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func verifySignature(r *http.Request, secret string, body []byte) bool {
	if secret == "" {
		return true
	}
	timestamp := r.Header.Get(headerTimestamp)
	signature := r.Header.Get(headerSignature)
	if timestamp == "" || signature == "" {
		return false
	}
	expected := signPayload(secret, timestamp, body)
	return hmac.Equal([]byte(expected), []byte(signature))
}
