package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

// RoleAdminUUID is the built-in admin role id (model.RoleAdminUUID).
const RoleAdminUUID = model.RoleAdminUUID

// User is a provisioned test user with an API token.
type User struct {
	Id       string
	Username string
	Email    string
	Password string
	Token    string
	Client   *apiclient.ApiClient
}

// ProvisionedServer couples a running server with its provisioned users.
type ProvisionedServer struct {
	Server *Server
	Admin  *User
}

// NewClient builds a JSON-content apiclient against the server. Accept is
// pinned to application/json: the server's content negotiation matches
// msgpack anywhere in the Accept header (not by q-order), and the default
// client Accept lists both — which would return msgpack that the json-tagged
// apiclient structs silently decode to zero values.
func NewClient(s *Server, token string) *apiclient.ApiClient {
	c, err := apiclient.NewClient(s.BaseURL, token, true)
	if err != nil {
		panic(fmt.Sprintf("harness: NewClient: %v", err))
	}
	c.SetContentType("application/json")
	c.GetRESTClient().SetAccept("application/json")
	c.SetTimeout(60 * time.Second)
	return c
}

// sessionClient builds a cookie-session client (for login-time flows).
func sessionClient(s *Server) *apiclient.ApiClient {
	c, err := apiclient.NewClient(s.BaseURL, "", true)
	if err != nil {
		panic(fmt.Sprintf("harness: sessionClient: %v", err))
	}
	c.SetContentType("application/json")
	c.GetRESTClient().SetAccept("application/json")
	c.UseSessionCookie(true)
	c.SetTimeout(60 * time.Second)
	return c
}

// ProvisionAdmin performs first-run setup against a fresh server: creates the
// admin user through the unauthenticated window, logs in, and mints an API
// token. It must be the first thing called against a new server.
func ProvisionAdmin(s *Server, username, password string) (*User, error) {
	body := map[string]interface{}{
		"username":        username,
		"email":           username + "@knot.test",
		"password":        password,
		"roles":           []string{RoleAdminUUID},
		"groups":          []string{},
		"active":          true,
		"max_spaces":      20,
		"compute_units":   20,
		"storage_units":   20,
		"max_tunnels":     10,
		"preferred_shell": "bash",
		"timezone":        "UTC",
	}
	data, _ := json.Marshal(body)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", s.BaseURL+"/api/users", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("first-run create admin: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		var e struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("first-run create admin: status %d: %s\nserver log tail:\n%s", resp.StatusCode, e.Error, s.LogTail(30))
	}

	return LoginUser(s, username, password)
}

// NewAnonClient builds an unauthenticated JSON client (login attempts, etc).
func NewAnonClient(s *Server) *apiclient.ApiClient {
	c, err := apiclient.NewClient(s.BaseURL, "", true)
	if err != nil {
		panic(fmt.Sprintf("harness: NewAnonClient: %v", err))
	}
	c.SetContentType("application/json")
	c.GetRESTClient().SetAccept("application/json")
	c.UseSessionCookie(true)
	c.SetTimeout(60 * time.Second)
	return c
}

// LoginUser logs in as an existing user and mints an API token, returning a
// ready-to-use client bundle.
func LoginUser(s *Server, username, password string) (*User, error) {
	sc := sessionClient(s)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	auth, code, err := sc.Login(ctx, username+"@knot.test", password, "")
	if err != nil {
		return nil, fmt.Errorf("login %s: %w (status %d)\nserver log tail:\n%s", username, err, code, s.LogTail(30))
	}
	sc.SetAuthToken(auth.Token)

	token, code, err := sc.CreateToken(ctx, "integration-test", nil)
	if err != nil {
		return nil, fmt.Errorf("create token for %s: %w (status %d)", username, err, code)
	}

	who, err := sc.WhoAmI(ctx)
	if err != nil {
		return nil, fmt.Errorf("whoami %s: %w", username, err)
	}

	return &User{
		Id:       who.Id,
		Username: username,
		Email:    username + "@knot.test",
		Password: password,
		Token:    token,
		Client:   NewClient(s, token),
	}, nil
}

// CreateUser creates a non-admin user via the admin client and logs them in.
// roles are role ids; permissions is a convenience to build a custom role
// when non-empty (a role named after the user is created and assigned).
func CreateUser(s *Server, admin *User, username string, permissions []uint16) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	roles := []string{}
	if len(permissions) > 0 {
		roleId, code, err := admin.Client.CreateRole(ctx, &apiclient.RoleRequest{
			Name:        "role-" + username,
			Permissions: permissions,
		})
		if err != nil {
			return nil, fmt.Errorf("create role for %s: %w (status %d)", username, err, code)
		}
		roles = append(roles, roleId)
	}

	req := &apiclient.CreateUserRequest{
		Username:       username,
		Password:       "Passw0rd!test",
		Email:          username + "@knot.test",
		Roles:          roles,
		Active:         true,
		MaxSpaces:      20,
		ComputeUnits:   20,
		StorageUnits:   20,
		MaxTunnels:     10,
		PreferredShell: "bash",
		Timezone:       "UTC",
	}
	if _, code, err := admin.Client.CreateUser(ctx, req); err != nil {
		return nil, fmt.Errorf("create user %s: %w (status %d)", username, err, code)
	}

	return LoginUser(s, username, "Passw0rd!test")
}

// TesterPermissions is the permission set given to standard non-admin users:
// enough to use spaces, run commands, copy files and read logs.
func TesterPermissions() []uint16 {
	return []uint16{
		model.PermissionUseSpaces,
		model.PermissionRunCommands,
		model.PermissionTransferSpaces,
		model.PermissionCopyFiles,
		model.PermissionUseLogs,
		model.PermissionUseWebTerminal,
		model.PermissionShareSpaces,
		model.PermissionManageOwnScripts,
		model.PermissionExecuteOwnScripts,
		model.PermissionManageOwnSkills,
		model.PermissionManageOwnSlashCommands,
		model.PermissionUseStackDefinitions,
		model.PermissionManageOwnStackDefinitions,
		model.PermissionManageEvents,
		model.PermissionUseLogSinks,
		model.PermissionUsePools,
		model.PermissionSetSpaceDependencies,
		model.PermissionUseSpaceStartupScript,
	}
}
