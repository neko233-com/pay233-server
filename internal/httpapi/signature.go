package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
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

func verifySignature(r *http.Request, secret string, body []byte, maxSkew time.Duration) error {
	if secret == "" {
		return nil
	}
	timestamp := r.Header.Get(headerTimestamp)
	signature := r.Header.Get(headerSignature)
	if timestamp == "" || signature == "" {
		return errors.New("missing request signature")
	}
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return errors.New("invalid request timestamp")
	}
	now := time.Now().UTC()
	if parsed.Before(now.Add(-maxSkew)) || parsed.After(now.Add(maxSkew)) {
		return errors.New("request timestamp outside allowed skew")
	}
	expected := signPayload(secret, timestamp, body)
	expectedBytes, _ := hex.DecodeString(expected)
	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return errors.New("invalid request signature")
	}
	if !hmac.Equal(expectedBytes, signatureBytes) {
		return errors.New("invalid request signature")
	}
	return nil
}
