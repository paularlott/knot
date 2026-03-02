package command_spaces

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
)

var CopyCmd = &cli.Command{
	Name:        "copy",
	Usage:       "Copy files between local machine and space",
	Description: "Copy files to or from a running space. Use spacename:path format for space files.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:         "workdir",
			Aliases:      []string{"w"},
			Usage:        "Working directory for relative paths in space",
			DefaultValue: "",
		},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "source",
			Required: true,
			Usage:    "Source file path (use spacename:path for space files)",
		},
		&cli.StringArg{
			Name:     "dest",
			Required: true,
			Usage:    "Destination file path (use spacename:path for space files)",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		workdir := cmd.GetString("workdir")
		source := cmd.GetStringArg("source")
		dest := cmd.GetStringArg("dest")

		// Determine direction and extract space name from the path with space: prefix
		var direction, localPath, spacePath, spaceName string
		var sourceSpaceName, sourceSpacePath, destSpaceName, destSpacePath string

		// Check if source has space: prefix (space name must be >1 char to avoid Windows drive letters)
		sourceColonIndex := strings.Index(source, ":")
		destColonIndex := strings.Index(dest, ":")

		sourceIsSpace := sourceColonIndex > 1
		destIsSpace := destColonIndex > 1

		if sourceIsSpace && destIsSpace {
			// Space to space copy
			direction = "space_to_space"
			sourceSpaceName = source[:sourceColonIndex]
			sourceSpacePath = source[sourceColonIndex+1:]
			destSpaceName = dest[:destColonIndex]
			destSpacePath = dest[destColonIndex+1:]
			if sourceSpacePath == "" {
				return fmt.Errorf("Source space path cannot be empty after '%s:'", sourceSpaceName)
			}
			if destSpacePath == "" {
				return fmt.Errorf("Destination space path cannot be empty after '%s:'", destSpaceName)
			}
		} else if sourceIsSpace {
			// Copy from space to local
			direction = "from_space"
			spaceName = source[:sourceColonIndex]
			spacePath = source[sourceColonIndex+1:]
			localPath = dest
			if spacePath == "" {
				return fmt.Errorf("Space path cannot be empty after '%s:'", spaceName)
			}
		} else if destIsSpace {
			// Copy from local to space
			direction = "to_space"
			spaceName = dest[:destColonIndex]
			spacePath = dest[destColonIndex+1:]
			localPath = source
			if spacePath == "" {
				return fmt.Errorf("Space path cannot be empty after '%s:'", spaceName)
			}
		} else {
			return fmt.Errorf("One path must use the format 'spacename:path' (space name must be more than 1 character)")
		}

		// Create API client
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Get the current user
		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		// Get a list of available spaces
		spaces, _, err := client.GetSpaces(context.Background(), user.Id)
		if err != nil {
			return fmt.Errorf("Error getting spaces: %w", err)
		}

		// Helper function to find space ID by name
		findSpaceId := func(name string) (string, error) {
			for _, space := range spaces.Spaces {
				if space.Name == name {
					return space.Id, nil
				}
			}
			return "", fmt.Errorf("Space not found: %s", name)
		}

		// Helper function to connect to a space websocket
		connectToSpace := func(spaceId string) (*websocket.Conn, error) {
			wsUrl := fmt.Sprintf("%s/space-io/%s/copy", cfg.WsServer, spaceId)
			header := http.Header{
				"Authorization": []string{fmt.Sprintf("Bearer %s", cfg.ApiToken)},
			}

			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cmd.GetBool("tls-skip-verify")}
			dialer.HandshakeTimeout = 5 * time.Second
			ws, response, err := dialer.Dial(wsUrl, header)
			if err != nil {
				if response != nil && response.StatusCode == http.StatusUnauthorized {
					return nil, fmt.Errorf("failed to authenticate with server, check remote token")
				} else if response != nil && response.StatusCode == http.StatusForbidden {
					return nil, fmt.Errorf("no permission to copy files in this space")
				}
				return nil, fmt.Errorf("Error connecting to websocket: %w", err)
			}
			return ws, nil
		}

		if direction == "space_to_space" {
			// Find both space IDs
			sourceSpaceId, err := findSpaceId(sourceSpaceName)
			if err != nil {
				return err
			}
			destSpaceId, err := findSpaceId(destSpaceName)
			if err != nil {
				return err
			}

			// Connect to source space
			sourceWs, err := connectToSpace(sourceSpaceId)
			if err != nil {
				return err
			}
			defer sourceWs.Close()

			// Connect to destination space
			destWs, err := connectToSpace(destSpaceId)
			if err != nil {
				return err
			}
			defer destWs.Close()

			// Read from source space
			sourceRequest := apiclient.CopyFileRequest{
				Direction: "from_space",
				SourcePath: sourceSpacePath,
				Workdir: workdir,
			}

			fmt.Printf("Copying %s:%s to %s:%s...\n", sourceSpaceName, sourceSpacePath, destSpaceName, destSpacePath)

			err = sourceWs.WriteJSON(sourceRequest)
			if err != nil {
				return fmt.Errorf("Error sending source copy request: %w", err)
			}

			var sourceResult map[string]interface{}
			err = sourceWs.ReadJSON(&sourceResult)
			if err != nil {
				return fmt.Errorf("Error reading source response: %w", err)
			}

			success, ok := sourceResult["success"].(bool)
			if !ok || !success {
				errorMsg, _ := sourceResult["error"].(string)
				return fmt.Errorf("Source read failed: %s", errorMsg)
			}

			// Extract content
			var content []byte
			if contentStr, ok := sourceResult["content"].(string); ok {
				content, err = base64.StdEncoding.DecodeString(contentStr)
				if err != nil {
					return fmt.Errorf("Error decoding file content: %w", err)
				}
			} else {
				return fmt.Errorf("Invalid content format in response")
			}

			// Write to destination space
			destRequest := apiclient.CopyFileRequest{
				Direction: "to_space",
				DestPath: destSpacePath,
				Content: content,
				Workdir: workdir,
			}

			err = destWs.WriteJSON(destRequest)
			if err != nil {
				return fmt.Errorf("Error sending destination copy request: %w", err)
			}

			var destResult map[string]interface{}
			err = destWs.ReadJSON(&destResult)
			if err != nil {
				return fmt.Errorf("Error reading destination response: %w", err)
			}

			success, ok = destResult["success"].(bool)
			if !ok || !success {
				errorMsg, _ := destResult["error"].(string)
				return fmt.Errorf("Destination write failed: %s", errorMsg)
			}

			fmt.Println("Copy completed successfully")
			return nil
		}

		// Handle local to space or space to local
		spaceId, err := findSpaceId(spaceName)
		if err != nil {
			return err
		}

		// Connect to the websocket for file copy
		ws, err := connectToSpace(spaceId)
		if err != nil {
			return err
		}
		defer ws.Close()

		var copyRequest apiclient.CopyFileRequest
		copyRequest.Direction = direction
		copyRequest.Workdir = workdir

		if direction == "to_space" {
			// Read local file
			content, err := os.ReadFile(localPath)
			if err != nil {
				return fmt.Errorf("Error reading local file: %w", err)
			}

			copyRequest.DestPath = spacePath
			copyRequest.Content = content

			fmt.Printf("Copying %s to %s:%s...\n", localPath, spaceName, spacePath)
		} else {
			// Copy from space
			copyRequest.SourcePath = spacePath
			fmt.Printf("Copying %s:%s to %s...\n", spaceName, spacePath, localPath)
		}

		// Send the copy request
		err = ws.WriteJSON(copyRequest)
		if err != nil {
			return fmt.Errorf("Error sending copy request: %w", err)
		}

		// Read the response
		var result map[string]interface{}
		err = ws.ReadJSON(&result)
		if err != nil {
			return fmt.Errorf("Error reading response: %w", err)
		}

		success, ok := result["success"].(bool)
		if !ok || !success {
			errorMsg, _ := result["error"].(string)
			return fmt.Errorf("Copy failed: %s", errorMsg)
		}

		if direction == "from_space" {
			// Write content to local file
			var content []byte
			if contentStr, ok := result["content"].(string); ok {
				// Decode base64 content
				var err error
				content, err = base64.StdEncoding.DecodeString(contentStr)
				if err != nil {
					return fmt.Errorf("Error decoding file content: %w", err)
				}
			} else {
				return fmt.Errorf("Invalid content format in response")
			}

			// Create directory if it doesn't exist
			localDir := filepath.Dir(localPath)
			if err := os.MkdirAll(localDir, 0755); err != nil {
				return fmt.Errorf("Error creating local directory: %w", err)
			}

			// Write file
			err = os.WriteFile(localPath, content, 0644)
			if err != nil {
				return fmt.Errorf("Error writing local file: %w", err)
			}
		}

		fmt.Println("Copy completed successfully")
		return nil
	},
}
