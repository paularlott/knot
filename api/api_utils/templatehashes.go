package api_utils

import (
	"sync"

	"github.com/paularlott/knot/database"

	"github.com/rs/zerolog/log"
)

var (
	templateHashMutex sync.RWMutex
	templateHashes    = make(map[string]string)
)

func LoadTemplateHashes() {
	log.Info().Msg("server: loading template hashes")

	db := database.GetInstance()

	// Load the template hashes from the database
	templateHashMutex.Lock()
	templates, err := db.GetTemplates()
	if err != nil {
		log.Fatal().Msgf("server: failed to load templates: %s", err.Error())
	}

	for _, template := range templates {
		templateHashes[template.Id] = template.Hash
	}

	templateHashMutex.Unlock()
}

func UpdateTemplateHash(templateId, hash string) {
	templateHashMutex.Lock()
	templateHashes[templateId] = hash
	templateHashMutex.Unlock()
}

func DeleteTemplateHash(templateId string) {
	templateHashMutex.Lock()
	delete(templateHashes, templateId)
	templateHashMutex.Unlock()
}

func GetTemplateHash(templateId string) string {
	templateHashMutex.RLock()
	hash, exists := templateHashes[templateId]
	templateHashMutex.RUnlock()

	if !exists {
		return ""
	}

	return hash
}

func GetTemplateHashes() map[string]string {
	templateHashMutex.RLock()
	hashes := make(map[string]string, len(templateHashes))
	for k, v := range templateHashes {
		hashes[k] = v
	}
	templateHashMutex.RUnlock()

	return hashes
}
