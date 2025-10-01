// Copyright 2024 The Score Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssertVersion_good(t *testing.T) {
	for _, tup := range [][2]string{
		{"=1.2.3", "v1.2.3"},
		{">=1.2.3", "v1.2.3"},
		{">=1.2.3", "v1.2.4"},
		{">1.2.3", "v1.2.4"},
		{">=1.1", "1.1.0"},
		{">=1.1", "1.2.0"},
		{">=1", "1.0.0"},
		{">1", "2.0.0"},
	} {
		t.Run(fmt.Sprintf("%v", tup), func(t *testing.T) {
			assert.NoError(t, AssertVersion(tup[0], tup[1]))
		})
	}
}

func TestAssertVersion_bad(t *testing.T) {
	for _, tup := range [][3]string{
		{"=1.2.3", "v1.2.0", "current version v1.2.0 does not match requested constraint =1.2.3"},
		{">2", "v1.2.0", "current version v1.2.0 does not match requested constraint >2"},
		{">1.2", "v1.2.0", "current version v1.2.0 does not match requested constraint >1.2"},
	} {
		t.Run(fmt.Sprintf("%v", tup), func(t *testing.T) {
			assert.EqualError(t, AssertVersion(tup[0], tup[1]), tup[2])
		})
	}
}

func TestSemverToI(t *testing.T) {
	validCases := []struct {
		version  string
		expected int
	}{
		{"1.2.3", 1002003},
		{"2.0.0", 2000000},
		{"0.9.1", 9001},
		{"1.2", 1002000},
		{"1", 1000000},
	}

	for _, tc := range validCases {
		t.Run(fmt.Sprintf("valid: %s", tc.version), func(t *testing.T) {
			result, err := semverToI(tc.version)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
	invalidCases := []string{
		"1.2.a",
		"a.b.c",
		"1..1",
		"1.2.3.4",
	}
	for _, version := range invalidCases {
		t.Run(fmt.Sprintf("invalid: %s", version), func(t *testing.T) {
			result, err := semverToI(version)
			assert.Error(t, err)
			assert.Equal(t, 0, result)
		})
	}
}
