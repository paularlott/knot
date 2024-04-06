package util

import (
	"bufio"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

func UpdateSSHConfig(sshConfig string) error {
	var lines []string

	log.Debug().Msg("Start updating .ssh/config")

	// If the file doesn't exist, create it
	if _, err := os.Stat(os.Getenv("HOME") + "/.ssh/config"); os.IsNotExist(err) {
		// Create the .ssh folder if it doesn't exist and make it private
		err := os.MkdirAll(os.Getenv("HOME")+"/.ssh", 0700)
		if err != nil {
			return err
		}
	} else {
		file, err := os.Open(os.Getenv("HOME") + "/.ssh/config")
		if err != nil {
			return err
		}
		defer file.Close()

		// Read the file line by line and strip out the current ssh config
		scanner := bufio.NewScanner(file)
		inBlock := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "#===KNOT-START===") {
				inBlock = true
			} else if strings.Contains(line, "#===KNOT-END===") {
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
		log.Debug().Msg("Adding ssh config to .ssh/config")

		lines = append(lines, "#===KNOT-START===")
		lines = append(lines, sshConfig)
		lines = append(lines, "#===KNOT-END===")
	}

	// Write lines to .ssh/config file
	file, err := os.OpenFile(os.Getenv("HOME")+"/.ssh/config", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, line := range lines {
		file.WriteString(line + "\n")
	}

	log.Debug().Msg("Done updating .ssh/config")

	return nil
}
