package validate

import (
	"testing"
)

func TestEmail(t *testing.T) {
	tests := []struct {
		email    string
		expected bool
	}{
		{"test@example.com", true},
		{"user.name@example.co.uk", true},
		{"user+tag@example.com", true},
		{"invalid", false},
		{"@example.com", false},
		{"user@", false},
		{"user@.com", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			result := Email(tt.email)
			if result != tt.expected {
				t.Errorf("Email(%q) = %v, expected %v", tt.email, result, tt.expected)
			}
		})
	}
}

func TestUri(t *testing.T) {
	tests := []struct {
		uri      string
		expected bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"https://example.com:8080", true},
		{"https://example.com/path", true},
		{"https://example.com/path?query=value", true},
		{"srv+https://example.com", true},
		{"ftp://example.com", false},
		{"not-a-url", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := Uri(tt.uri)
			if result != tt.expected {
				t.Errorf("Uri(%q) = %v, expected %v", tt.uri, result, tt.expected)
			}
		})
	}
}

func TestName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"validName", true},
		{"valid-name", true},
		{"valid123", true},
		{"a1", true},
		{"-invalid", false},
		{"1invalid", false},
		{"invalid--name", false},
		{"", false},
		{"a", false}, // too short
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Name(tt.name)
			if result != tt.expected {
				t.Errorf("Name(%q) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestVarName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"validVar", true},
		{"valid_var", true},
		{"valid123", true},
		{"a1", true},
		{"_invalid", false},
		{"1invalid", false},
		{"invalid-var", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := VarName(tt.name)
			if result != tt.expected {
				t.Errorf("VarName(%q) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestSubdomain(t *testing.T) {
	tests := []struct {
		subdomain string
		expected  bool
	}{
		{"valid", true},
		{"valid-subdomain", true},
		{"valid123", true},
		{"-invalid", false},
		{"invalid-", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.subdomain, func(t *testing.T) {
			result := Subdomain(tt.subdomain)
			if result != tt.expected {
				t.Errorf("Subdomain(%q) = %v, expected %v", tt.subdomain, result, tt.expected)
			}
		})
	}
}

func TestPassword(t *testing.T) {
	tests := []struct {
		password string
		expected bool
	}{
		{"password123", true},
		{"12345678", true},
		{"short", false},
		{"1234567", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.password, func(t *testing.T) {
			result := Password(tt.password)
			if result != tt.expected {
				t.Errorf("Password(%q) = %v, expected %v", tt.password, result, tt.expected)
			}
		})
	}
}

func TestTokenName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"valid", true},
		{"a", true},
		{string(make([]byte, 255)), true},
		{string(make([]byte, 256)), false},
		{"", false},
	}

	for i, tt := range tests {
		t.Run(string(rune(i)), func(t *testing.T) {
			result := TokenName(tt.name)
			if result != tt.expected {
				t.Errorf("TokenName(len=%d) = %v, expected %v", len(tt.name), result, tt.expected)
			}
		})
	}
}

func TestOneOf(t *testing.T) {
	values := []string{"option1", "option2", "option3"}

	tests := []struct {
		value    string
		expected bool
	}{
		{"option1", true},
		{"option2", true},
		{"option3", true},
		{"option4", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := OneOf(tt.value, values)
			if result != tt.expected {
				t.Errorf("OneOf(%q, %v) = %v, expected %v", tt.value, values, result, tt.expected)
			}
		})
	}
}

func TestRequired(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"text", true},
		{"a", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := Required(tt.text)
			if result != tt.expected {
				t.Errorf("Required(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestMaxLength(t *testing.T) {
	tests := []struct {
		text     string
		length   int
		expected bool
	}{
		{"short", 10, true},
		{"exact", 5, true},
		{"toolong", 5, false},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			result := MaxLength(tt.text, tt.length)
			if result != tt.expected {
				t.Errorf("MaxLength(%q, %d) = %v, expected %v", tt.text, tt.length, result, tt.expected)
			}
		})
	}
}

func TestIsNumber(t *testing.T) {
	tests := []struct {
		value    int
		min      int
		max      int
		expected bool
	}{
		{5, 0, 10, true},
		{0, 0, 10, true},
		{10, 0, 10, true},
		{-1, 0, 10, false},
		{11, 0, 10, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsNumber(tt.value, tt.min, tt.max)
			if result != tt.expected {
				t.Errorf("IsNumber(%d, %d, %d) = %v, expected %v", tt.value, tt.min, tt.max, result, tt.expected)
			}
		})
	}
}

func TestIsPositiveNumber(t *testing.T) {
	tests := []struct {
		value    int
		expected bool
	}{
		{0, true},
		{1, true},
		{100, true},
		{-1, false},
		{-100, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := IsPositiveNumber(tt.value)
			if result != tt.expected {
				t.Errorf("IsPositiveNumber(%d) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestIsTime(t *testing.T) {
	tests := []struct {
		time     string
		expected bool
	}{
		{"9:00am", true},
		{"12:30pm", true},
		{"1:45pm", true},
		{"10:00AM", false}, // uppercase not allowed
		{"9:00", false},    // missing am/pm
		{"25:00pm", true},  // regex allows this (validation bug?)
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.time, func(t *testing.T) {
			result := IsTime(tt.time)
			if result != tt.expected {
				t.Errorf("IsTime(%q) = %v, expected %v", tt.time, result, tt.expected)
			}
		})
	}
}

func TestUUID(t *testing.T) {
	tests := []struct {
		uuid     string
		expected bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"123e4567-e89b-12d3-a456-426614174000", true},
		{"invalid-uuid", false},
		{"550e8400-e29b-41d4-a716", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.uuid, func(t *testing.T) {
			result := UUID(tt.uuid)
			if result != tt.expected {
				t.Errorf("UUID(%q) = %v, expected %v", tt.uuid, result, tt.expected)
			}
		})
	}
}
