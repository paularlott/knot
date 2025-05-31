package web

import (
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/pelletier/go-toml/v2"
)

// Icon represents a single icon entry
type Icon struct {
	Description string `toml:"description" json:"description"`
	Url         string `toml:"url" json:"url"`
}

type IconList struct {
	Icons []Icon `toml:"icons"`
}

func loadIcons() []Icon {
	var iconList []Icon

	iconFiles := viper.GetStringSlice("server.ui.icons")
	for _, iconFile := range iconFiles {
		log.Info().Msgf("Loading icons from file: %s", iconFile)

		// If file doesn't exist, skip it
		_, err := os.Stat(iconFile)
		if err != nil {
			log.Warn().Msgf("Icon file %s does not exist, skipping", iconFile)
			continue
		}

		// Load the icons from the .toml file
		file, err := os.Open(iconFile)
		if err != nil {
			log.Warn().Msgf("Failed to open icon file %s: %v", iconFile, err)
			continue
		}

		// Read the data from the file
		iconData, err := os.ReadFile(iconFile)
		if err != nil {
			log.Warn().Msgf("Failed to read icon file %s: %v", iconFile, err)
			file.Close()
			continue
		}

		file.Close()

		var iconsFromFile IconList
		if err := toml.Unmarshal(iconData, &iconsFromFile); err != nil {
			log.Warn().Msgf("Failed to unmarshal icons from %s: %v", iconFile, err)
			continue
		}

		iconList = append(iconList, iconsFromFile.Icons...)
	}

	return iconList
}
