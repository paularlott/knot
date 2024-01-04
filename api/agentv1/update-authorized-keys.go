package agentv1

import (
	"bufio"
	"net/http"
	"os"
	"strings"

	"github.com/paularlott/knot/util/rest"
	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
)

type AgentUpdateAuthorizedKeysRequest struct {
  Key string `json:"key"`
}

type AgentUpdateAuthorizedKeysResponse struct {
  Status bool `json:"status"`
}

var (
  lastPublicSSHKey string = ""
)

func HandleAgentUpdateAuthorizedKeys(w http.ResponseWriter, r *http.Request) {
  request := AgentUpdateAuthorizedKeysRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if viper.GetBool("agent.update-authorized-keys") && viper.GetInt("agent.port.ssh") > 0 {
    // If the key is the same as the last key then skip
    if request.Key != lastPublicSSHKey {
      log.Debug().Msg("updating authorized_keys")

      lastPublicSSHKey = request.Key
      err = updateAuthorizedKeys(request.Key)
      if err != nil {
        log.Debug().Msgf("failed to update authorized_keys: %s", err)
      }
    } else {
      log.Debug().Msg("authorized_keys already up to date")
    }
  }

  rest.SendJSON(http.StatusOK, w, AgentUpdateAuthorizedKeysResponse{
    Status: true,
  })
}

func CallAgentUpdateAuthorizedKeys(client *rest.RESTClient, sshKey string) bool {
  response := &AgentUpdateAuthorizedKeysResponse{}
  statusCode, err := client.Post(
    "/update-authorized-keys",
    AgentUpdateAuthorizedKeysRequest{
      Key: sshKey,
    },
    response,
    http.StatusOK,
  )
  return statusCode == http.StatusOK && err == nil && response.Status
}

func updateAuthorizedKeys(key string) (error) {
  var lines []string
  keyFound := false

  // If the file doesn't exist, create it
  if _, err := os.Stat(os.Getenv("HOME") + "/.ssh/authorized_keys"); os.IsNotExist(err) {
    // Create the .ssh folder if it doesn't exist and make it private
    err := os.MkdirAll(os.Getenv("HOME") + "/.ssh", 0700)
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
        break;
      }
    }

    if err := scanner.Err(); err != nil {
      return err
    }
  }

  // If key not found add it to lines
  if !keyFound {
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
  }

  return nil
}