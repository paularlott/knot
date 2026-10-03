package filestore

import (
	"errors"
	"testing"
)

func TestValidShortName(t *testing.T) {
	for name, want := range map[string]bool{
		"abc":                             true,
		"my-bucket":                       true,
		"a1b2":                            true,
		"abcdefghijklmnopqrstuvwxyz1234":  true, // 30
		"ab":                              false,
		"abcdefghijklmnopqrstuvwxyz12345": false, // 31
		"my--bucket":                      false,
		"-abc":                            false,
		"abc-":                            false,
		"My-Bucket":                       false,
		"my.bucket":                       false,
		"my_bucket":                       false,
	} {
		if got := ValidShortName(name); got != want {
			t.Errorf("ValidShortName(%q) = %v", name, got)
		}
	}
}

func TestNamespacedNames(t *testing.T) {
	p := &Principal{UserId: "u1", Username: "Paul.Arlott"}

	if got, err := NewBucketName(p, "config"); err != nil || got != "paul.arlott--config" {
		t.Errorf("NewBucketName short: %q %v", got, err)
	}
	if got, err := NewBucketName(p, "paul.arlott--config"); err != nil || got != "paul.arlott--config" {
		t.Errorf("NewBucketName own full: %q %v", got, err)
	}
	if _, err := NewBucketName(p, "bob--config"); !errors.Is(err, ErrBucketNamespace) {
		t.Errorf("NewBucketName other namespace: %v", err)
	}
	if _, err := NewBucketName(p, "Config"); !errors.Is(err, ErrInvalidShortName) {
		t.Errorf("NewBucketName invalid: %v", err)
	}
	if _, err := FullName("abcdefghijklmnopqrstuvwxyz12345678", "abcdefghijklmnopqrstuvwxyz1234"); !errors.Is(err, ErrNameTooLong) {
		t.Errorf("FullName too long: %v", err)
	}

	if got := ResolveName(p, "config"); got != "paul.arlott--config" {
		t.Errorf("ResolveName short: %q", got)
	}
	if got := ResolveName(p, "bob--config"); got != "bob--config" {
		t.Errorf("ResolveName full: %q", got)
	}
	if got := DisplayName(p, "paul.arlott--config"); got != "config" {
		t.Errorf("DisplayName own: %q", got)
	}
	if got := DisplayName(p, "bob--config"); got != "bob--config" {
		t.Errorf("DisplayName other: %q", got)
	}
	if got := ShortName("bob--config"); got != "config" {
		t.Errorf("ShortName: %q", got)
	}

	for name, want := range map[string]bool{
		"bob--config": true,
		"a--b--c":     false,
		"bob---x":     false,
		"bob.-x":      false,
		"bob..x":      false,
	} {
		if got := ValidBucketName(name); got != want {
			t.Errorf("ValidBucketName(%q) = %v", name, got)
		}
	}
}

func TestFullNameUnusableUsername(t *testing.T) {
	for _, username := range []string{"paul-", "a.-b", "x..y"} {
		if _, err := FullName(username, "notes"); !errors.Is(err, ErrUsernameUnusable) {
			t.Errorf("FullName(%q): %v", username, err)
		}
	}
}
