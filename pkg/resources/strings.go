package resources

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

//ShortenString Cut string size, but maintain a reference to the original string using a hash of the full string in the result
func ShortenString(s string, n int) string {
	hashLen := 4
	anReg, err := buildAlphanumRegexp()
	if err != nil {
		return ""
	}

	s = anReg.ReplaceAllString(s, "")
	if len(s) < n {
		return s
	}
	// +1 to account for the hyphen
	postfixLen := hashLen + 1
	cutSize := n - postfixLen

	if n < (hashLen + 1) {
		n = len(s)
		cutSize = len(s)
	}
	cutStr := s[0:cutSize]

	hasher := sha256.New()
	if _, err := hasher.Write([]byte(s)); err != nil {
		return ""
	}
	hashedStr := base32.StdEncoding.EncodeToString(hasher.Sum(nil))

	return strings.ToLower(fmt.Sprintf("%s-%s", cutStr, hashedStr[0:hashLen]))
}

func buildAlphanumRegexp() (*regexp.Regexp, error) {
	regexpStr := "[^a-zA-Z0-9]+"
	anReg, err := regexp.Compile(regexpStr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to compile regexp %s", regexpStr)
	}
	return anReg, nil
}

// StringOrDefault checks string and returns given default string if empty
func StringOrDefault(str, defaultTo string) string {
	if str == "" {
		return defaultTo
	}
	return str
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
