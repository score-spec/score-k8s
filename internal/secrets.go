package internal

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	magicPrefix = "🔐💬"
	magicSuffix = "💬🔐"
)

func EncodeSecretReference(namespace, secret, key string) string {
	return magicPrefix + base64.RawURLEncoding.EncodeToString([]byte(namespace)) +
		"." + base64.RawURLEncoding.EncodeToString([]byte(secret)) +
		"." + base64.RawURLEncoding.EncodeToString([]byte(key)) +
		magicSuffix
}

type SecretRef struct {
	Namespace string
	Name      string
	Key       string
}

// DecodeSecretReferences resolves a string in a kubernetes manifest that may contain secret references into a
// useful string or reference to the real secret itself.
// These can be embedded in:
// - environment variables
// - file mounts where the file contains only the secret
func DecodeSecretReferences(source string) ([]string, []SecretRef, error) {
	parts := strings.Split(source, magicPrefix)
	output := make([]SecretRef, 0, len(parts))
	for i, part := range parts[1:] {
		si := strings.Index(part, magicSuffix)
		if si >= 0 {
			r := part[:si]
			parts[i] = part[si+2:]
			bits := strings.Split(r, ".")
			if len(bits) != 3 {
				return nil, nil, errors.Errorf("invalid secret ref: more than 3 parts")
			}
			out := SecretRef{}
			if r, err := base64.RawURLEncoding.DecodeString(bits[0]); err != nil {
				return nil, nil, errors.Errorf("invalid secret ref: failed to decode parts.0")
			} else {
				out.Namespace = string(r)
			}
			if r, err := base64.RawURLEncoding.DecodeString(bits[1]); err != nil {
				return nil, nil, errors.Errorf("invalid secret ref: failed to decode parts.1")
			} else {
				out.Name = string(r)
			}
			if r, err := base64.RawURLEncoding.DecodeString(bits[2]); err != nil {
				return nil, nil, errors.Errorf("invalid secret ref: failed to decode parts.2")
			} else {
				out.Key = string(r)
			}
			output = append(output, out)
		}
	}
	return parts, output, nil
}

func FindFirstUnresolvedSecretRef(path string, v interface{}) (string, bool) {
	switch typed := v.(type) {
	case string:
		pp := strings.Index(typed, magicPrefix)
		sp := strings.Index(typed, magicSuffix)
		return path, pp >= 0 && sp > 0 && sp > pp
	case map[string]interface{}:
		for s, i := range typed {
			if p, ok := FindFirstUnresolvedSecretRef(path+"."+s, i); ok {
				return p, true
			}
		}
	case []interface{}:
		for s, i := range typed {
			if p, ok := FindFirstUnresolvedSecretRef(path+"."+strconv.Itoa(s), i); ok {
				return p, true
			}
		}
	}
	return "", false
}
