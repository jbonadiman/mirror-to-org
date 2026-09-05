package httperr

import (
	"fmt"
	"strings"
)

// CredentialError aborts the whole run, not just one reference.
type CredentialError struct {
	Message string
}

func (e *CredentialError) Error() string { return e.Message }

func CheckStatus(statusCode int, body []byte, action string, isTarget bool) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	detail := strings.TrimSpace(string(body))
	if len(detail) > 500 {
		detail = detail[:500]
	}
	if detail == "" {
		detail = "no detail"
	}
	message := fmt.Sprintf("%s failed (%d): %s", action, statusCode, detail)
	if isTarget && (statusCode == 401 || statusCode == 403) {
		return &CredentialError{Message: message}
	}
	return fmt.Errorf("%s", message)
}
