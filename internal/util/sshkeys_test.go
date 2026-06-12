package util

import (
	"reflect"
	"testing"
)

func TestSplitSSHPublicKeys(t *testing.T) {
	keys := " ssh-ed25519 AAAA user@example.com \n\nssh-rsa BBBB other@example.com\n"

	got := SplitSSHPublicKeys(keys)
	want := []string{
		"ssh-ed25519 AAAA user@example.com",
		"ssh-rsa BBBB other@example.com",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitSSHPublicKeys() = %#v, want %#v", got, want)
	}
}
