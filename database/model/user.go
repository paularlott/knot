package model

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// User object
type User struct {
  Id string `json:"user_id"`
  Username string `json:"username"`
  Email string `json:"email"`
  Password string `json:"password"`
  SSHPublicKey string `json:"ssh_public_key"`
  Roles []string `json:"roles"`
  Active bool `json:"active"`
  PreferredShell string `json:"preferred_shell"`
  LastLoginAt time.Time `json:"last_login_at"`
  UpdatedAt time.Time `json:"updated_at"`
  CreatedAt time.Time `json:"created_at"`
}

func NewUser(username string, email string, password string, roles []string) *User {
  user := &User{
    Id: uuid.New().String(),
    Username: username,
    Email: email,
    Active: true,
    SSHPublicKey: "",
    PreferredShell: "zsh",
    Roles: roles,
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

func (u *User) HasPermission(permission int) bool {
  for _, role := range u.Roles {

    // If role exists in rolePermissions map then check if the permission belongs to the role
    if permissions, ok := rolePermissions[role]; ok {
      for _, p := range permissions {
        if p == permission {
          return true
        }
      }
    }
  }

  return true
}
