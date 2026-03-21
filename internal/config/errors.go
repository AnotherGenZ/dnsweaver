package config

import (
	"fmt"
	"strings"
)

// ConfigError provides a structured configuration error with actionable guidance.
// It includes the field that caused the error, what went wrong, how to fix it,
// and an example of a valid value.
type ConfigError struct {
	// Field identifies the configuration field (e.g., "DNSWEAVER_LOG_LEVEL" or "providers[0].url").
	Field string

	// Message describes what is wrong.
	Message string

	// Help provides guidance on how to fix the error.
	Help string

	// Example shows a valid value for this field.
	Example string
}

// Error implements the error interface with a human-readable, multi-line message.
func (e *ConfigError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s", e.Field, e.Message)
	if e.Help != "" {
		fmt.Fprintf(&b, "\n  %s", e.Help)
	}
	if e.Example != "" {
		fmt.Fprintf(&b, "\n  Example: %s", e.Example)
	}
	return b.String()
}

// configErr creates a ConfigError with just field and message (most common case).
func configErr(field, message string) *ConfigError {
	return &ConfigError{Field: field, Message: message}
}

// configErrHelp creates a ConfigError with field, message, and help text.
func configErrHelp(field, message, help string) *ConfigError {
	return &ConfigError{Field: field, Message: message, Help: help}
}

// configErrFull creates a ConfigError with all fields populated.
func configErrFull(field, message, help, example string) *ConfigError {
	return &ConfigError{Field: field, Message: message, Help: help, Example: example}
}

// ValidationError represents one or more configuration validation errors.
type ValidationError struct {
	Errors []*ConfigError
}

// Error implements the error interface, formatting all errors as a readable list.
func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return fmt.Sprintf("configuration error:\n  %s", e.Errors[0].Error())
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d configuration errors:", len(e.Errors))
	for i, err := range e.Errors {
		fmt.Fprintf(&b, "\n  %d) %s", i+1, err.Error())
	}
	return b.String()
}
