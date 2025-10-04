package model

import (
	"testing"
	"time"
)

func TestNewToken(t *testing.T) {
	token := NewToken("test-token", "user-123")

	if token.Id == "" {
		t.Error("Token ID should not be empty")
	}
	if token.Name != "test-token" {
		t.Errorf("Expected name 'test-token', got '%s'", token.Name)
	}
	if token.UserId != "user-123" {
		t.Errorf("Expected user ID 'user-123', got '%s'", token.UserId)
	}
	if token.IsDeleted {
		t.Error("New token should not be deleted")
	}
	if token.ExpiresAfter.Before(time.Now()) {
		t.Error("Token should not be expired on creation")
	}

	expectedExpiry := time.Now().Add(MaxTokenAge)
	if token.ExpiresAfter.Before(expectedExpiry.Add(-1*time.Minute)) || token.ExpiresAfter.After(expectedExpiry.Add(1*time.Minute)) {
		t.Error("Token expiry should be approximately MaxTokenAge from now")
	}
}
