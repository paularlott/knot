package util

import (
	"testing"
)

func TestInArray(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item exists",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "banana",
			expected: true,
		},
		{
			name:     "item does not exist",
			slice:    []string{"apple", "banana", "cherry"},
			item:     "grape",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "apple",
			expected: false,
		},
		{
			name:     "empty item",
			slice:    []string{"apple", "banana"},
			item:     "",
			expected: false,
		},
		{
			name:     "empty item in slice",
			slice:    []string{"apple", "", "banana"},
			item:     "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InArray(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("InArray(%v, %q) = %v, expected %v", tt.slice, tt.item, result, tt.expected)
			}
		})
	}
}

func TestConvertToBytes(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  int64
		expectErr bool
	}{
		{
			name:      "gigabytes",
			input:     "2G",
			expected:  2 * 1024 * 1024 * 1024,
			expectErr: false,
		},
		{
			name:      "megabytes",
			input:     "512M",
			expected:  512 * 1024 * 1024,
			expectErr: false,
		},
		{
			name:      "kilobytes",
			input:     "1024K",
			expected:  1024 * 1024,
			expectErr: false,
		},
		{
			name:      "bytes with B",
			input:     "1024B",
			expected:  1024,
			expectErr: false,
		},
		{
			name:      "bytes without suffix",
			input:     "2048",
			expected:  2048,
			expectErr: false,
		},
		{
			name:      "lowercase g",
			input:     "1g",
			expected:  1 * 1024 * 1024 * 1024,
			expectErr: false,
		},
		{
			name:      "invalid format",
			input:     "invalid",
			expected:  0,
			expectErr: true,
		},
		{
			name:      "invalid number",
			input:     "abcG",
			expected:  0,
			expectErr: true,
		},
		{
			name:      "empty string",
			input:     "",
			expected:  0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToBytes(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Errorf("ConvertToBytes(%q) expected error but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ConvertToBytes(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("ConvertToBytes(%q) = %d, expected %d", tt.input, result, tt.expected)
				}
			}
		})
	}
}
