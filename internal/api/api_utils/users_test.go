package api_utils

import (
	"testing"
)

func TestNewApiUtilsUsers(t *testing.T) {
	utils := NewApiUtilsUsers()
	if utils == nil {
		t.Fatal("NewApiUtilsUsers returned nil")
	}
}
