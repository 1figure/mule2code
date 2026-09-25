package contacts

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidEmail is the error of <validation:is-email ... message="Missing or invalid email">.
// Its text is the message attribute, copied verbatim because it ends up in the errors file.
var ErrInvalidEmail = errors.New("Missing or invalid email")

// Same shape the Mule Validation module accepts: local part, one '@', dotted domain with a 2+ letter TLD.
var emailPattern = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+/=?^_`{|}~.-]+@[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*\\.[A-Za-z]{2,}$")

// IsEmail reports whether email is a syntactically valid address.
func IsEmail(email string) bool {
	if strings.TrimSpace(email) == "" || len(email) > 254 {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at > 64 || email[0] == '.' || email[at-1] == '.' || strings.Contains(email, "..") {
		return false
	}
	return emailPattern.MatchString(email)
}

// ValidateEmail is <validation:is-email email="#[payload.email]" message="Missing or invalid email">,
// the per-record processor of main-processing-step. It returns ErrInvalidEmail when the row's
// email cell is missing or not an address.
func ValidateEmail(row Row) error {
	if !IsEmail(row["email"]) {
		return ErrInvalidEmail
	}
	return nil
}
