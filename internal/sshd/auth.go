package sshd

import (
	"sync"

	"github.com/paularlott/knot/internal/util"

	"github.com/gliderlabs/ssh"
	"github.com/paularlott/knot/internal/log"
	gossh "golang.org/x/crypto/ssh"
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
			authKeys = append(authKeys, util.SplitSSHPublicKeys(key)...)
		}
	}

	authorizedKeysMutex.Lock()
	defer authorizedKeysMutex.Unlock()
	authorizedKeys = authKeys

	return nil
}

// AuthResultFunc, when set, is called with the outcome of every public-key
// authentication attempt, the key's SHA256 fingerprint and the client
// address. The agent wires it to report attempts to the knot servers; a
// client offering several keys produces one call per key.
var AuthResultFunc func(success bool, fingerprint, remoteAddr string)

func publicKeyHandler(ctx ssh.Context, key ssh.PublicKey) bool {
	log.Debug("testing public key")

	ok := authorizeKey(key)
	if AuthResultFunc != nil {
		AuthResultFunc(ok, gossh.FingerprintSHA256(key), ctx.RemoteAddr().String())
	}
	return ok
}

func authorizeKey(key ssh.PublicKey) bool {
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
