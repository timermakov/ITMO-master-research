package envcfg

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Required returns the environment variable value or an error if unset/empty.
func Required(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("required environment variable %q is not set", key)
	}
	return v, nil
}

// RequiredInt parses a required int environment variable.
func RequiredInt(key string) (int, error) {
	s, err := Required(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("environment variable %q: %w", key, err)
	}
	return n, nil
}

// RequiredInt64 parses a required int64 environment variable.
func RequiredInt64(key string) (int64, error) {
	s, err := Required(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("environment variable %q: %w", key, err)
	}
	return n, nil
}

// RequiredBool parses a required bool environment variable (true/false).
func RequiredBool(key string) (bool, error) {
	s, err := Required(key)
	if err != nil {
		return false, err
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, fmt.Errorf("environment variable %q: %w", key, err)
	}
	return b, nil
}

// RequiredDurationFromSeconds parses seconds from env into time.Duration.
func RequiredDurationFromSeconds(key string) (time.Duration, error) {
	sec, err := RequiredInt(key)
	if err != nil {
		return 0, err
	}
	return time.Duration(sec) * time.Second, nil
}

// RequiredCSV splits a required comma-separated env value.
func RequiredCSV(key string) ([]string, error) {
	s, err := Required(key)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("environment variable %q is empty after split", key)
	}
	return out, nil
}
