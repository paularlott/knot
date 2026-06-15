package command_spaces

import "testing"

func TestParseCustomFields(t *testing.T) {
	fields, err := parseCustomFields([]string{
		"env=dev",
		"token=a=b=c",
		" owner =paul",
	})
	if err != nil {
		t.Fatalf("parseCustomFields returned error: %v", err)
	}

	expected := map[string]string{
		"env":   "dev",
		"token": "a=b=c",
		"owner": "paul",
	}

	if len(fields) != len(expected) {
		t.Fatalf("expected %d custom fields, got %d", len(expected), len(fields))
	}

	for _, field := range fields {
		if expected[field.Name] != field.Value {
			t.Fatalf("expected %q to be %q, got %q", field.Name, expected[field.Name], field.Value)
		}
	}
}

func TestParseCustomFieldsRejectsMalformedFields(t *testing.T) {
	tests := []struct {
		name   string
		fields []string
	}{
		{name: "missing separator", fields: []string{"env"}},
		{name: "blank name", fields: []string{"=dev"}},
		{name: "whitespace name", fields: []string{"  =dev"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseCustomFields(tt.fields); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
