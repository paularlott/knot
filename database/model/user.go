package model

import (
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

// User object
type User struct {
	Id              string     `json:"user_id" db:"user_id,pk"`
	Username        string     `json:"username" db:"username"`
	Email           string     `json:"email" db:"email"`
	Password        string     `json:"password" db:"password"`
	TOTPSecret      string     `json:"totp_secret" db:"totp_secret"`
	ServicePassword string     `json:"service_password" db:"service_password"`
	SSHPublicKey    string     `json:"ssh_public_key" db:"ssh_public_key"`
	GitHubUsername  string     `json:"github_username" db:"github_username"`
	Roles           []string   `json:"roles" db:"roles,json"`
	Groups          []string   `json:"groups" db:"groups,json"`
	Active          bool       `json:"active" db:"active"`
	MaxSpaces       uint32     `json:"max_spaces" db:"max_spaces"`
	ComputeUnits    uint32     `json:"compute_units" db:"compute_units"`
	StorageUnits    uint32     `json:"storage_units" db:"storage_units"`
	MaxTunnels      uint32     `json:"max_tunnels" db:"max_tunnels"`
	PreferredShell  string     `json:"preferred_shell" db:"preferred_shell"`
	Timezone        string     `json:"timezone" db:"timezone"`
	LastLoginAt     *time.Time `json:"last_login_at" db:"last_login_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

type Usage struct {
	ComputeUnits                   uint32
	StorageUnits                   uint32
	NumberSpaces                   int
	NumberSpacesDeployed           int
	NumberSpacesDeployedInLocation int
}

type Quota struct {
	ComputeUnits uint32
	StorageUnits uint32
	MaxSpaces    uint32
	MaxTunnels   uint32
}

func NewUser(username string, email string, password string, roles []string, groups []string, sshPublicKey string, preferredShell string, timezone string, maxSpaces uint32, githubUsername string, computeUnits uint32, storageUnits uint32, maxTunnels uint32) *User {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	now := time.Now().UTC()

	user := &User{
		Id:              id.String(),
		Username:        username,
		Email:           email,
		Active:          true,
		SSHPublicKey:    sshPublicKey,
		GitHubUsername:  githubUsername,
		PreferredShell:  preferredShell,
		Roles:           roles,
		Groups:          groups,
		Timezone:        timezone,
		MaxSpaces:       maxSpaces,
		ComputeUnits:    computeUnits,
		StorageUnits:    storageUnits,
		MaxTunnels:      maxTunnels,
		ServicePassword: generateRandomString(16),
		UpdatedAt:       now,
		CreatedAt:       now,
	}

	user.SetPassword(password)

	return user
}

// Set the password for the user
func (u *User) SetPassword(password string) error {
	// Create bcrypt password
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err == nil {
		u.Password = string(bytes)
	}

	return err
}

// Check the password for the user
func (u *User) CheckPassword(password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil
}

func (u *User) HasPermission(permission uint16) bool {
	for _, role := range u.Roles {

		// If role exists in rolePermissions map then check if the permission belongs to the role
		if r, ok := roleCache[role]; ok {
			for _, p := range r.Permissions {
				if p == permission {
					return true
				}
			}
		}
	}

	return false
}

func (u *User) HasAnyGroup(groups *JSONDbArray) bool {

	// If user has no groups then return false
	if len(u.Groups) == 0 {
		return false
	}

	// If user has groups then check if any of the groups match
	for _, group := range u.Groups {
		for _, g := range *groups {
			if g == group {
				return true
			}
		}
	}

	return false
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}
