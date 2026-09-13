package service

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// ValidateKvmSpaceAddress checks that a KVM space's chosen IP is valid for
// the template's network: inside the configured range, not a reserved
// address, and not already taken by another space in the same zone. An
// ignoreSpaceId lets the caller skip the space being validated (reserved
// for future re-validation paths; create passes "").
//
// The template's own network fields are validated at template create/update
// time; a parse failure here means the template changed since, and is
// reported as an error rather than silently ignored.
func ValidateKvmSpaceAddress(template *model.Template, ipAddress string, ignoreSpaceId string) error {
	network, err := model.ParseKvmNetwork(template)
	if err != nil {
		return err
	}

	if err := network.ValidateSpaceIP(strings.TrimSpace(ipAddress)); err != nil {
		return err
	}

	db := database.GetInstance()
	spaces, err := db.GetSpaces()
	if err != nil {
		return err
	}

	ip := strings.TrimSpace(ipAddress)
	for _, space := range spaces {
		if space.IsDeleted || space.Id == ignoreSpaceId || space.IPAddress == "" {
			continue
		}
		if space.IPAddress == ip {
			return fmt.Errorf("IP address %s is already in use by space %s", ip, space.Name)
		}
	}

	return nil
}
