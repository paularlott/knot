package validate

import (
	"regexp"
	"strings"
)

func Email(email string) bool {
	re := regexp.MustCompile(`(?i)^([a-z0-9](?:[a-z0-9&'+=_\.-]+)?)@([a-z0-9_-]+)(\.[a-z0-9_-]+)*(\.[a-z]{2,})+$`)
	return re.MatchString(email)
}

func Uri(uri string) bool {
	re := regexp.MustCompile(`(?i)^(srv\+)?https?:\/\/([\w-]+:[\w-]+@)?[\w-]+(?:\.[\w-]+)*(?::\d+)?(?:\/(?:[\w~%()=.,+-]|%[0-9A-Fa-f]{2})*)*(?:\?(?:[\w~%()=.,+-]|%[0-9A-Fa-f]{2})+=(?:[\w~%()=.,+-]|%[0-9A-Fa-f]{2})*(?:&(?:[\w~%()=.,+-]|%[0-9A-Fa-f]{2})+=(?:[\w~%()=.,+-]|%[0-9A-Fa-f]{2})*)*)?(#[\w~%()=.,\/+-]*)?$`)
	return re.MatchString(uri)
}

func Name(name string) bool {
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-]{1,63}$`)
	return re.MatchString(name) && !strings.Contains(name, "--")
}

func VarName(name string) bool {
	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{1,63}$`)
	return re.MatchString(name)
}

func Subdomain(subdomain string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-]{0,62}[a-zA-Z0-9]$`)
	return re.MatchString(subdomain)
}

func Password(password string) bool {
	return len(password) >= 8
}

func TokenName(name string) bool {
	return len(name) > 0 && len(name) <= 255
}

func OneOf(value string, values []string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}

func Required(text string) bool {
	return len(text) > 0
}

func MaxLength(text string, length int) bool {
	return len(text) <= length
}

func IsNumber(value int, min int, max int) bool {
	return value >= min && value <= max
}

func IsPositiveNumber(value int) bool {
	return value >= 0
}

func IsTime(time string) bool {
	re := regexp.MustCompile(`^\d{1,2}:\d{2}[ap]m$`)
	return re.MatchString(time)
}

func UUID(uuid string) bool {
	re := regexp.MustCompile(`^^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	return re.MatchString(uuid)
}
