package sshd

import (
	"sync"

	"github.com/paularlott/knot/internal/util"

	"github.com/gliderlabs/ssh"
	"github.com/paularlott/knot/internal/log"
)

var (
	authorizedKeysMutex = sync.RWMutex{}
	authorizedKeys      = []string{}
)

func UpdateAuthorizedKeys(keys []string, githubUsernames []string) error {
	var authKeys = []string{}

	// If the github username is not empty, then download the keys from github
	if len(githubUsernames) > 0 {
		log.Debug("Downloading keys from GitHub")
		for _, githubUsername := range githubUsernames {
			githubKeys, err := util.GetGitHubKeysArray(githubUsername)
			if err != nil {
				return err
			}

			authKeys = append(authKeys, githubKeys...)
		}
	}

	if len(keys) > 0 {
		log.Debug("Adding key")
		for _, key := range keys {
			authKeys = append(authKeys, key)
		}
	}

	authorizedKeysMutex.Lock()
	defer authorizedKeysMutex.Unlock()
	authorizedKeys = authKeys

	return nil
}

func publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	log.Debug("testing public key")

	authorizedKeysMutex.RLock()
	defer authorizedKeysMutex.RUnlock()

	for _, authorizedKey := range authorizedKeys {
		parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
		if err == nil && ssh.KeysEqual(key, parsedKey) {
			log.Debug("key found in authorized keys")
			return true
		}
	}

	log.Debug("key not found in authorized keys")

	return false
}
