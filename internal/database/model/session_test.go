package model

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("User-Agent", "TestAgent/1.0")

	session := NewSession(req, "user-123")

	if session.Id == "" {
		t.Error("Session ID should not be empty")
	}
	if session.UserId != "user-123" {
		t.Errorf("Expected user ID 'user-123', got '%s'", session.UserId)
	}
	if session.Ip != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got '%s'", session.Ip)
	}
	if session.UserAgent != "TestAgent/1.0" {
		t.Errorf("Expected user agent 'TestAgent/1.0', got '%s'", session.UserAgent)
	}
	if session.IsDeleted {
		t.Error("New session should not be deleted")
	}
	if session.ExpiresAfter.Before(time.Now()) {
		t.Error("Session should not be expired on creation")
	}
}

func TestNewSessionWithXForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.1:443")

	session := NewSession(req, "user-456")

	if session.Ip != "203.0.113.1" {
		t.Errorf("Expected IP from X-Forwarded-For '203.0.113.1', got '%s'", session.Ip)
	}
}
