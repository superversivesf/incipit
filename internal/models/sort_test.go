package models

import "testing"

func TestSortTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Expanse", "Expanse, The"},
		{"A Brief History of Time", "Brief History of Time, A"},
		{"An Ocean of Night", "Ocean of Night, An"},
		{"Leviathan Wakes", "Leviathan Wakes"},
		{"", ""},
	}
	for _, tt := range tests {
		got := SortTitle(tt.input)
		if got != tt.expected {
			t.Errorf("SortTitle(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
