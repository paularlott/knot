package model

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestPropertiesForwardedChain(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spaces", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1:34567")
	req.Header.Set("User-Agent", "knot-cli/1.0")

	props := *RequestProperties(req, nil)

	if props["source_ip"] != "203.0.113.9" {
		t.Errorf("source_ip should be the first forwarded hop without port, got %v", props["source_ip"])
	}
	if props["x_forwarded_for"] != "203.0.113.9, 10.0.0.1:34567" {
		t.Errorf("x_forwarded_for should preserve the full raw chain, got %v", props["x_forwarded_for"])
	}
	if props["user_agent"] != "knot-cli/1.0" {
		t.Errorf("unexpected user_agent: %v", props["user_agent"])
	}
}

func TestRequestPropertiesNoForward(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spaces", nil)
	req.RemoteAddr = "192.168.1.10:52311"
	req.Header.Set("User-Agent", "test")

	props := *RequestProperties(req, nil)

	if props["source_ip"] != "192.168.1.10" {
		t.Errorf("source_ip should fall back to the remote address without port, got %v", props["source_ip"])
	}
	if _, present := props["x_forwarded_for"]; present {
		t.Errorf("x_forwarded_for should be absent without the header, got %v", props["x_forwarded_for"])
	}
}
