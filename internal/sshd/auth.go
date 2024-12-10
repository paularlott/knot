package sshd

import (
	"sync"

	"github.com/paularlott/knot/util"

	"github.com/gliderlabs/ssh"
	"github.com/rs/zerolog/log"
)

var (
	authorizedKeysMutex = sync.RWMutex{}
	authorizedKeys      = []string{}
)

func UpdateAuthorizedKeys(key string, githubUsername string) error {
	var authKeys = []string{}

	// If the github username is not empty, then download the keys from github
	if githubUsername != "" {
		log.Debug().Msg("Downloading keys from GitHub")
		githubKeys, err := util.GetGitHubKeysArray(githubUsername)
		if err != nil {
			return err
		}

		authKeys = append(authKeys, githubKeys...)
	}

	if key != "" {
		log.Debug().Msg("Adding key")
		authKeys = append(authKeys, key)
	}

	authorizedKeysMutex.Lock()
	defer authorizedKeysMutex.Unlock()
	authorizedKeys = authKeys

	return nil
}

func publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	log.Debug().Msg("sshd: testing public key")

	authorizedKeysMutex.RLock()
	defer authorizedKeysMutex.RUnlock()

	for _, authorizedKey := range authorizedKeys {
		parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
		if err == nil && ssh.KeysEqual(key, parsedKey) {
			log.Debug().Msg("sshd: key found in authorized keys")
			return true
		}
	}

	log.Debug().Msg("sshd: key not found in authorized keys")

	return false
}
