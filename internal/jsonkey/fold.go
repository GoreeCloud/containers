// Package jsonkey detects aliases accepted by encoding/json's case-insensitive
// struct-field matching. Preview policy rejects ambiguous keys in every object.
package jsonkey

import (
	"strings"
	"unicode"
)

// Fold returns a representative of each Unicode simple-fold equivalence class.
// Use only on object member names, never values or environment variable names.
func Fold(key string) string {
	return strings.Map(func(r rune) rune {
		min := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < min {
				min = next
			}
		}
		return min
	}, key)
}
