package util

import (
	"bufio"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

func UpdateAuthorizedKeys(key string) error {
	var lines []string
	keyFound := false

	log.Debug().Msg("Start updating authorized_keys")

	// If the file doesn't exist, create it
	if _, err := os.Stat(os.Getenv("HOME") + "/.ssh/authorized_keys"); os.IsNotExist(err) {
		// Create the .ssh folder if it doesn't exist and make it private
		err := os.MkdirAll(os.Getenv("HOME")+"/.ssh", 0700)
		if err != nil {
			return err
		}
	} else {
		file, err := os.Open(os.Getenv("HOME") + "/.ssh/authorized_keys")
		if err != nil {
			return err
		}
		defer file.Close()

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
			} else if inBlock && line == key {
				// key already exists
				keyFound = true
				break
			}
		}

		if err := scanner.Err(); err != nil {
			return err
		}
	}

	// If key not found add it to lines
	if !keyFound {
		log.Debug().Msg("Adding key to authorized_keys")

		lines = append(lines, "#===KNOT-START===")
		lines = append(lines, key)
		lines = append(lines, "#===KNOT-END===")

		// Write lines to authorized_keys file
		file, err := os.OpenFile(os.Getenv("HOME")+"/.ssh/authorized_keys", os.O_CREATE|os.O_WRONLY, 0700)
		if err != nil {
			return err
		}
		defer file.Close()

		for _, line := range lines {
			file.WriteString(line + "\n")
		}
	} else {
		log.Debug().Msg("Key already in authorized_keys")
	}

	log.Debug().Msg("Done updating authorized_keys")

	return nil
}
