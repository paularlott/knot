package util

import (
	"bufio"
	"strings"
)

func SplitSSHPublicKeys(keys string) []string {
	var sshKeys []string

	scanner := bufio.NewScanner(strings.NewReader(keys))
	for scanner.Scan() {
		key := strings.TrimSpace(scanner.Text())
		if key != "" {
			sshKeys = append(sshKeys, key)
		}
	}

	return sshKeys
}
