// Copyright 2024 Humanitec
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	magicPrefix = "🔐💬"
	magicSuffix = "💬🔐"
)

// EncodeSecretReference encodes a reference to a specific value within a secret.
// An encoded value might look like: 🔐💬my.secret_inner-key"💬🔐.
// Note the rules on valid characters here https://kubernetes.io/docs/concepts/configuration/secret/#restriction-names-data.
func EncodeSecretReference(secret, key string) string {
	return magicPrefix + secret + "_" + key + magicSuffix
}

type SecretRef struct {
	Name string
	Key  string
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
			parts[1+i] = part[si+len(magicSuffix):]
			bits := strings.SplitN(r, "_", 2)
			if len(bits) != 2 {
				return nil, nil, errors.Errorf("invalid secret ref: doesn't contain _")
			}
			out := SecretRef{
				Name: bits[0],
				Key:  bits[1],
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
