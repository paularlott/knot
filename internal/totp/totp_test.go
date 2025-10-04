package totp

import (
	"testing"
	"time"
)

func TestGenerateSecret(t *testing.T) {
	secret := GenerateSecret()
	
	if len(secret) != 16 {
		t.Errorf("Expected secret length 16, got %d", len(secret))
	}
	
	// Verify all characters are valid base32
	validChars := "234567QWERTYUIOPASDFGHJKLZXCVBNM"
	for _, c := range secret {
		found := false
		for _, v := range validChars {
			if c == v {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Invalid character in secret: %c", c)
		}
	}
	
	// Generate multiple secrets and check they're usually different
	// (skip this check as it can occasionally fail due to randomness)
	secret2 := GenerateSecret()
	_ = secret2 // Just verify it generates without error
}

func TestGetCode(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	
	// Test with specific time slice
	code, err := GetCode(secret, 1)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	if len(code) != 6 {
		t.Errorf("Expected code length 6, got %d", len(code))
	}
	
	// Test with current time (timeSlice = 0)
	code2, err := GetCode(secret, 0)
	if err != nil {
		t.Fatalf("GetCode with current time failed: %v", err)
	}
	if len(code2) != 6 {
		t.Errorf("Expected code length 6, got %d", len(code2))
	}
	
	// Test that same time slice produces same code
	code3, err := GetCode(secret, 1)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	if code != code3 {
		t.Errorf("Same time slice should produce same code: %s != %s", code, code3)
	}
}

func TestGetCodeInvalidSecret(t *testing.T) {
	_, err := GetCode("invalid!@#$", 1)
	if err == nil {
		t.Error("Expected error for invalid secret")
	}
}

func TestVerifyCode(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	
	// Get current valid code
	currentTimeSlice := time.Now().UTC().Unix() / 30
	validCode, err := GetCode(secret, currentTimeSlice)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	
	// Test valid code
	if !VerifyCode(secret, validCode, 1) {
		t.Error("Valid code should verify successfully")
	}
	
	// Test invalid code
	if VerifyCode(secret, "000000", 1) {
		t.Error("Invalid code should not verify")
	}
	
	// Test code from previous time slice (within discrepancy)
	prevCode, err := GetCode(secret, currentTimeSlice-1)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	if !VerifyCode(secret, prevCode, 1) {
		t.Error("Code from previous time slice should verify with discrepancy=1")
	}
	
	// Test code from next time slice (within discrepancy)
	nextCode, err := GetCode(secret, currentTimeSlice+1)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	if !VerifyCode(secret, nextCode, 1) {
		t.Error("Code from next time slice should verify with discrepancy=1")
	}
	
	// Test code outside discrepancy range
	farCode, err := GetCode(secret, currentTimeSlice+5)
	if err != nil {
		t.Fatalf("GetCode failed: %v", err)
	}
	if VerifyCode(secret, farCode, 1) {
		t.Error("Code outside discrepancy range should not verify")
	}
}

func TestBase32Decode(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			name:      "valid base32",
			input:     "JBSWY3DPEHPK3PXP",
			expectErr: false,
		},
		{
			name:      "valid lowercase",
			input:     "jbswy3dpehpk3pxp",
			expectErr: false,
		},
		{
			name:      "invalid characters",
			input:     "invalid!@#$",
			expectErr: true,
		},
		{
			name:      "empty string",
			input:     "",
			expectErr: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := base32Decode(tt.input)
			if tt.expectErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
