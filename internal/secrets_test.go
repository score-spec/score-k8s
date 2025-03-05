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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeSecretReferences_nominal(t *testing.T) {
	splits, refs, err := DecodeSecretReferences(
		EncodeSecretReference("s1", "k1") + "thing" +
			EncodeSecretReference("s2", "k2") +
			EncodeSecretReference("a.val1d-dns.subdomain", "a-val1d.k_y"),
	)
	assert.NoError(t, err)
	assert.Equal(t, []string{"", "thing", "", ""}, splits)
	assert.Equal(t, []SecretRef{
		{Name: "s1", Key: "k1"},
		{Name: "s2", Key: "k2"},
		{Name: "a.val1d-dns.subdomain", Key: "a-val1d.k_y"},
	}, refs)
}
