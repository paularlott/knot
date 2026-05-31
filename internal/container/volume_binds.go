package container

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

func ValidateManagedVolumeBinds(binds []string, volumeData model.VolumeDataMap) error {
	for _, bind := range binds {
		parts := strings.SplitN(bind, ":", 2)
		if len(parts) != 2 {
			continue
		}

		source := strings.TrimSpace(parts[0])
		if source == "" || strings.HasPrefix(source, "/") || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") {
			continue
		}
		if _, ok := volumeData[source]; ok {
			continue
		}

		return fmt.Errorf("volume bind %q references undeclared named volume %q; add it to template volumes or use a host path", bind, source)
	}

	return nil
}
