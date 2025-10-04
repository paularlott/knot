package crypt

import (
	"encoding/base64"
	"testing"
)

func TestGenerateAPIKey(t *testing.T) {
	key1, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}
	
	if key1 == "" {
		t.Error("Generated API key should not be empty")
	}
	
	// Verify it's valid base64
	_, err = base64.URLEncoding.DecodeString(key1)
	if err != nil {
		t.Errorf("Generated API key is not valid base64: %v", err)
	}
	
	// Generate another key to ensure uniqueness
	key2, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}
	
	if key1 == key2 {
		t.Error("Generated API keys should be unique")
	}
}

func TestCreateKey(t *testing.T) {
	key := CreateKey()
	
	if len(key) != 32 {
		t.Errorf("Expected key length 32, got %d", len(key))
	}
	
	// Verify all characters are alphanumeric
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for _, c := range key {
		found := false
		for _, v := range validChars {
			if c == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Invalid character in key: %c", c)
		}
	}
	
	// Generate another key to ensure uniqueness
	key2 := CreateKey()
	if key == key2 {
		t.Error("Generated keys should be unique")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := CreateKey()
	plaintext := "Hello, World!"
	
	encrypted := Encrypt(key, plaintext)
	if encrypted == plaintext {
		t.Error("Encrypted text should differ from plaintext")
	}
	
	decrypted := Decrypt(key, encrypted)
	if decrypted != plaintext {
		t.Errorf("Decrypted text doesn't match original. Expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptB64DecryptB64(t *testing.T) {
	key := CreateKey()
	plaintext := "Test message with special chars: !@#$%^&*()"
	
	encrypted := EncryptB64(key, plaintext)
	if encrypted == plaintext {
		t.Error("Encrypted text should differ from plaintext")
	}
	
	// Verify it's valid base64
	_, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Errorf("Encrypted text is not valid base64: %v", err)
	}
	
	decrypted := DecryptB64(key, encrypted)
	if decrypted != plaintext {
		t.Errorf("Decrypted text doesn't match original. Expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptDecryptEmptyString(t *testing.T) {
	key := CreateKey()
	plaintext := ""
	
	encrypted := Encrypt(key, plaintext)
	decrypted := Decrypt(key, encrypted)
	
	if decrypted != plaintext {
		t.Errorf("Decrypted empty string doesn't match. Expected %q, got %q", plaintext, decrypted)
	}
}

func TestEncryptDecryptLongText(t *testing.T) {
	key := CreateKey()
	plaintext := "This is a longer text message that contains multiple sentences. " +
		"It should be encrypted and decrypted correctly regardless of length. " +
		"Testing with various characters: 1234567890 !@#$%^&*() αβγδε"
	
	encrypted := Encrypt(key, plaintext)
	decrypted := Decrypt(key, encrypted)
	
	if decrypted != plaintext {
		t.Errorf("Decrypted long text doesn't match original")
	}
}

func TestDecryptB64ShortInput(t *testing.T) {
	key := CreateKey()
	shortB64 := base64.StdEncoding.EncodeToString([]byte("short"))
	
	result := DecryptB64(key, shortB64)
	if result != "" {
		t.Errorf("Expected empty string for short input, got %q", result)
	}
}
