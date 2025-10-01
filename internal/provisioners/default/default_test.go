// Copyright 2024 The Score Authors
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

package _default

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/score-spec/score-k8s/internal/provisioners/loader"
)

func TestDefaultProvisioners(t *testing.T) {
	p, err := loader.LoadProvisioners([]byte(DefaultProvisioners))
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Greater(t, len(p), 0)
}
