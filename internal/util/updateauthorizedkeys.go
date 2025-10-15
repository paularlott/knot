package util

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/internal/log"
)

func UpdateAuthorizedKeys(keys []string, githubUsernames []string) error {
	var lines []string
	var combinedKeys string = ""

	log.Debug("Start updating authorized_keys")

	// Merge the keys into a single string
	for _, key := range keys {
		combinedKeys += key + "\n"
	}

	// If the github username is not empty, then download the keys from github
	log.Debug("Downloading keys from GitHub")
	for _, githubUsername := range githubUsernames {
		githubKeys, err := GetGitHubKeys(githubUsername)
		if err != nil {
			return err
		}
		combinedKeys += githubKeys + "\n"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// If the file doesn't exist, create it
	if _, err := os.Stat(filepath.Join(home, ".ssh", "authorized_keys")); os.IsNotExist(err) {
		// Create the .ssh folder if it doesn't exist and make it private
		err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700)
		if err != nil {
			return err
		}
	} else {
		file, err := os.Open(filepath.Join(home, ".ssh", "authorized_keys"))
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
			}
		}

		if err := scanner.Err(); err != nil {
			return err
		}
	}

	// If keys then add them to the authorized_keys
	if combinedKeys != "" {
		log.Debug("Adding key to authorized_keys")

		lines = append(lines, "#===KNOT-START===")
		lines = append(lines, combinedKeys)
		lines = append(lines, "#===KNOT-END===")
	}

	// Write lines to authorized_keys file
	file, err := os.OpenFile(filepath.Join(home, ".ssh", "authorized_keys"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
	if err != nil {
		return err
	}
	defer file.Close()

	for _, line := range lines {
		file.WriteString(line + "\n")
	}

	log.Debug("Done updating authorized_keys")

	return nil
}

// Download the public keys from GitHub, https://github.com/<username>.keys
func GetGitHubKeys(username string) (string, error) {
	log.Debug("Downloading keys from GitHub for", "username", username)

	// Download the keys from GitHub
	resp, err := http.Get("https://github.com/" + username + ".keys")
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func GetGitHubKeysArray(username string) ([]string, error) {
	keys := []string{}

	ghKeys, err := GetGitHubKeys(username)
	if err != nil {
		return keys, err
	}

	scanner := bufio.NewScanner(strings.NewReader(ghKeys))
	for scanner.Scan() {
		keys = append(keys, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return keys, err
	}

	return keys, nil
}
