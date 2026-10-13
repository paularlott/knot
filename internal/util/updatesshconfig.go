package util

import (
	"bufio"
	"os"
	"strings"

	"github.com/paularlott/knot/internal/log"
)

// blockSuffix is the suffix on the markers of an alias's knot block.
func blockSuffix(alias string) string {
	if alias == "default" {
		return ""
	}
	return " (" + alias + ")"
}

// ReadSSHConfigAgentForwarding reads the knot block for alias from
// ~/.ssh/config and reports, for each host in it, whether agent forwarding is
// on. A missing file or block gives an empty map.
func ReadSSHConfigAgentForwarding(alias string) (map[string]bool, error) {
	hosts := map[string]bool{}

	home, err := os.UserHomeDir()
	if err != nil {
		return hosts, err
	}

	file, err := os.Open(home + "/.ssh/config")
	if err != nil {
		if os.IsNotExist(err) {
			return hosts, nil
		}
		return hosts, err
	}
	defer file.Close()

	alias = blockSuffix(alias)
	scanner := bufio.NewScanner(file)
	inBlock := false
	host := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "#===KNOT-START"+alias+"===") {
			inBlock = true
			continue
		} else if strings.Contains(line, "#===KNOT-END"+alias+"===") {
			break
		} else if !inBlock {
			continue
		}

		// ssh_config allows "Key value" or "Key=value"
		fields := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == '=' })
		if len(fields) < 2 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "host":
			host = fields[1]
			hosts[host] = false
		case "forwardagent":
			if host != "" {
				hosts[host] = strings.EqualFold(fields[1], "yes")
			}
		}
	}

	return hosts, scanner.Err()
}

func UpdateSSHConfig(sshConfig string, alias string) error {
	var lines []string

	alias = blockSuffix(alias)

	log.Debug("Start updating .ssh/config")

	// Get the users home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// If the file doesn't exist, create it
	if _, err := os.Stat(home + "/.ssh/config"); os.IsNotExist(err) {
		// Create the .ssh folder if it doesn't exist and make it private
		err := os.MkdirAll(home+"/.ssh", 0700)
		if err != nil {
			return err
		}
	} else {
		file, err := os.Open(home + "/.ssh/config")
		if err != nil {
			return err
		}
		defer file.Close()

		// Read the file line by line and strip out the current ssh config
		scanner := bufio.NewScanner(file)
		inBlock := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "#===KNOT-START"+alias+"===") {
				inBlock = true
			} else if strings.Contains(line, "#===KNOT-END"+alias+"===") {
				inBlock = false
			} else if !inBlock {
				lines = append(lines, line)
			}
		}

		if err := scanner.Err(); err != nil {
			return err
		}
	}

	// If new config given
	if sshConfig != "" {
		log.Debug("Adding ssh config to .ssh/config")

		lines = append(lines, "#===KNOT-START"+alias+"===")
		lines = append(lines, sshConfig)
		lines = append(lines, "#===KNOT-END"+alias+"===")
	}

	// Write lines to .ssh/config file
	file, err := os.OpenFile(home+"/.ssh/config", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, line := range lines {
		file.WriteString(line + "\n")
	}

	log.Debug("Done updating .ssh/config")

	return nil
}
