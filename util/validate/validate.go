package validate

import "regexp"

func Email(email string) bool {
  re := regexp.MustCompile(`(?i)^([a-z0-9](?:[a-z0-9&'+=_\.-]+)?)@([a-z0-9_-]+)(\.[a-z0-9_-]+)*(\.[a-z]{2,})+$`)
  return re.MatchString(email)
}

func Username(username string) bool {
  re := regexp.MustCompile(`^[a-z0-9]{1,32}$`);
  return re.MatchString(username);
}

func Password(password string) bool {
  return len(password) >= 8
}