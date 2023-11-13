package validate

import "regexp"

func Email(email string) bool {
  re := regexp.MustCompile(`(?i)^([a-z0-9](?:[a-z0-9&'+=_\.-]+)?)@([a-z0-9_-]+)(\.[a-z0-9_-]+)*(\.[a-z]{2,})+$`)
  return re.MatchString(email)
}

func Uri(uri string) bool {
  re := regexp.MustCompile(`(?i)^((srv\+)?(http|https):\/\/)?([\w-]+:[\w-]+@)?[\w-]+(?:\.[\w-]+)*(?::\d+)?(?:\/[\w ~\\\(\)%\=\.,\+-]*)*(?:\?[\w ~\\\(\)%\=\.,\+-]+=[\w ~\\\(\)%\=\.,\+-]*(?:&[\w ~\\\(\)%\=\.,\+-]+=[\w ~\\\(\)%\=\.,\+-]*)*)?(#[\w ~\\\(\)%\=\.,\/\+-]*)?$`)
  return re.MatchString(uri)
}

func Username(username string) bool {
  re := regexp.MustCompile(`^[a-z0-9]{1,32}$`);
  return re.MatchString(username);
}

func Password(password string) bool {
  return len(password) >= 8
}

func TokenName(name string) bool {
  return len(name) > 0 && len(name) <= 255
}
