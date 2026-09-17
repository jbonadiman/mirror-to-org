package httperr

import (
	"errors"
	"fmt"
	"strings"
)

// maxDetailBytes caps how much of a response body goes into an error.
const maxDetailBytes = 500

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
	if len(detail) > maxDetailBytes {
		detail = detail[:maxDetailBytes]
	}
	if detail == "" {
		detail = "no detail"
	}
	message := fmt.Sprintf("%s failed (%d): %s", action, statusCode, detail)
	if isTarget && (statusCode == 401 || statusCode == 403) {
		return &CredentialError{Message: message}
	}
	return errors.New(message)
}
