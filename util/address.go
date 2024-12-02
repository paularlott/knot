package util

import (
	"fmt"
	"net"
	"strconv"
)

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

// Get the local IP address of the machine we're running on, interfaces are checked for being up.
func GetLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		// Skip down and loopback interfaces
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			// Check if the IP is an IPv4 address and not a loopback address
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no connected network interfaces found")
}
