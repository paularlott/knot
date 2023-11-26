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
  Active bool `json:"active"`
  IsAdmin bool `json:"is_admin"`
  LastLoginAt time.Time `json:"last_login_at"`
  UpdatedAt time.Time `json:"updated_at"`
  CreatedAt time.Time `json:"created_at"`
}

func NewUser(username string, email string, password string, isAdmin bool) *User {
  user := &User{
    Id: uuid.New().String(),
    Username: username,
    Email: email,
    Active: true,
    IsAdmin: isAdmin,
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
