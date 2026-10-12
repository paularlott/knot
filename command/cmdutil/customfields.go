package cmdutil

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
)

// ParseCustomFields parses --custom-field values given as name=value.
func ParseCustomFields(rawFields []string) ([]apiclient.CustomFieldValue, error) {
	customFields := make([]apiclient.CustomFieldValue, 0, len(rawFields))

	for _, rawField := range rawFields {
		name, value, ok := strings.Cut(rawField, "=")
		if !ok {
			return nil, fmt.Errorf("invalid custom field %q, expected name=value", rawField)
		}

		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("invalid custom field %q, name is required", rawField)
		}

		customFields = append(customFields, apiclient.CustomFieldValue{
			Name:  name,
			Value: value,
		})
	}

	return customFields, nil
}
