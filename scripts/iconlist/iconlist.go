package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Icon represents a single icon entry for TOML output
type Icon struct {
	Description string `toml:"description"`
	Url         string `toml:"url"`
}

type IconList struct {
	Icons []Icon `toml:"icons"`
}

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: iconlist <source_dir> <url_root> <output_file>")
		os.Exit(1)
	}
	sourceDir := os.Args[1]
	urlRoot := os.Args[2]
	outputFile := os.Args[3]

	var icons []Icon
	extensions := map[string]bool{".png": true, ".webp": true, ".jpg": true, ".svg": true}

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if extensions[ext] {
			filename := info.Name()
			description := strings.TrimSuffix(filename, ext)
			url := strings.TrimRight(urlRoot, "/") + "/" + filename
			icons = append(icons, Icon{Description: description, Url: url})
		}
		return nil
	})
	if err != nil {
		fmt.Println("Error scanning directory:", err)
		os.Exit(1)
	}

	iconList := IconList{Icons: icons}
	f, err := os.Create(outputFile)
	if err != nil {
		fmt.Println("Error creating output file:", err)
		os.Exit(1)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(iconList); err != nil {
		fmt.Println("Error writing TOML:", err)
		os.Exit(1)
	}
}
