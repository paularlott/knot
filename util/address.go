package util

import "strconv"

func FixListenAddress(address string) string {
	if address == "" {
		return ""
	}

	// If the address is just numbers then assume it's a port and prefix with a colon
	if _, err := strconv.Atoi(address); err == nil {
		return ":" + address
	}

	return address
}
