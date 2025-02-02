package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"fmt"
	"strings"
	"time"

	"golang.org/x/exp/rand"
)

// Creates a 16 digit secret key in base32 format
func GenerateSecret() string {
	b32Chars := "234567QWERTYUIOPASDFGHJKLZXCVBNM"
	rand.Seed(uint64(time.Now().UTC().UnixNano()))
	secret := make([]byte, 16)
	for i := range secret {
		secret[i] = b32Chars[rand.Intn(len(b32Chars))]
	}
	return string(secret)
}

// Calculates the 6 digit TOTP code
func GetCode(secret string, timeSlice int64) (string, error) {
	if timeSlice == 0 {
		timeSlice = time.Now().UTC().Unix() / 30
	}
	key, err := base32Decode(secret)
	if err != nil {
		return "", err
	}
	var b [8]byte
	// ...existing code...
	for i := 4; i < 8; i++ {
		b[i] = byte(timeSlice >> (56 - 8*i))
	}
	h := hmac.New(sha1.New, key)
	h.Write(b[:])
	hash := h.Sum(nil)
	offset := hash[len(hash)-1] & 0x0F
	truncatedHash := (int(hash[offset])&0x7F)<<24 |
		(int(hash[offset+1])&0xFF)<<16 |
		(int(hash[offset+2])&0xFF)<<8 |
		(int(hash[offset+3]) & 0xFF)
	code := truncatedHash % 1000000
	return fmt.Sprintf("%06d", code), nil
}

// Checks if the provided code is valid within allowed drift
func VerifyCode(secret, code string, discrepancy int) bool {
	currentTimeSlice := time.Now().UTC().Unix() / 30
	for i := -discrepancy; i <= discrepancy; i++ {
		fmt.Println(i)
		c, err := GetCode(secret, currentTimeSlice+int64(i))
		if err == nil && c == code {
			return true
		}
	}
	return false
}

// Decodes a base32 string into bytes
func base32Decode(s string) ([]byte, error) {
	s = strings.ToUpper(s)
	data, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid base32 string: %v", err)
	}
	return data, nil
}
