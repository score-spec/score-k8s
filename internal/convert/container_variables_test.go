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

package convert

import (
	"testing"

	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"

	"github.com/score-spec/score-k8s/internal"
)

func Test_generateSecretRefEnvVarName(t *testing.T) {
	assert.Equal(t, "__ref_bGInLge7AUJiuCF1YpXFjQ", generateSecretRefEnvVarName("", ""))
	assert.Equal(t, "__ref_7nSlDImL8R05KcUzKJ00KQ", generateSecretRefEnvVarName("hello", "world"))
	assert.Equal(t, "__ref_pCu17eplmVon10uSLW8i8A", generateSecretRefEnvVarName("hello", "dan"))
}

func noSubstitutesFunction(s string) (string, error) {
	panic("should not be called")
}

func Test_convertContainerVariable_0(t *testing.T) {
	out, err := convertContainerVariable("KEY", "VALUE", noSubstitutesFunction)
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.EnvVar{{Name: "KEY", Value: "VALUE", ValueFrom: nil}}, out)
}

func Test_convertContainerVariable_1_sub(t *testing.T) {
	out, err := convertContainerVariable("KEY", "${foo.bar}", func(s string) (string, error) {
		return "?", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.EnvVar{{Name: "KEY", Value: "?", ValueFrom: nil}}, out)
}

func Test_convertContainerVariable_N_subs(t *testing.T) {
	out, err := convertContainerVariable("KEY", "x${foo.bar}y${a.b}", func(s string) (string, error) {
		return "?", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.EnvVar{{Name: "KEY", Value: "x?y?", ValueFrom: nil}}, out)
}

func Test_convertContainerVariable_1_secret(t *testing.T) {
	out, err := convertContainerVariable("KEY", "${foo.bar}", func(s string) (string, error) {
		return internal.EncodeSecretReference("default", "some-key"), nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.EnvVar{{Name: "KEY", ValueFrom: &coreV1.EnvVarSource{
		SecretKeyRef: &coreV1.SecretKeySelector{
			LocalObjectReference: coreV1.LocalObjectReference{Name: "default"},
			Key:                  "some-key",
		},
	}}}, out)
}

func Test_convertContainerVariable_2_secret(t *testing.T) {
	out, err := convertContainerVariable("KEY", "${foo.bar} ${a.b}", func(s string) (string, error) {
		return map[string]string{
			"foo.bar": internal.EncodeSecretReference("default", "some-key"),
			"a.b":     internal.EncodeSecretReference("default", "other-key"),
		}[s], nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.EnvVar{
		{Name: "__ref_0960osB2KjQY08QKfHliCg", ValueFrom: &coreV1.EnvVarSource{
			SecretKeyRef: &coreV1.SecretKeySelector{
				LocalObjectReference: coreV1.LocalObjectReference{Name: "default"},
				Key:                  "some-key",
			},
		}},
		{Name: "__ref_mWObImRl7lfuP04NHDPsvA", ValueFrom: &coreV1.EnvVarSource{
			SecretKeyRef: &coreV1.SecretKeySelector{
				LocalObjectReference: coreV1.LocalObjectReference{Name: "default"},
				Key:                  "other-key",
			},
		}},
		{Name: "KEY", Value: "$(__ref_0960osB2KjQY08QKfHliCg) $(__ref_mWObImRl7lfuP04NHDPsvA)"},
	}, out)
}

func Test_convertContainerVariables_sorting(t *testing.T) {
	out, err := convertContainerVariables(map[string]string{
		"BUZZ": "FIZZ",
		"KEY":  "${foo.bar} ${a.b}",
		"FIZZ": "BUZZ",
	}, func(s string) (string, error) {
		return map[string]string{
			"foo.bar": internal.EncodeSecretReference("default", "some-key"),
			"a.b":     internal.EncodeSecretReference("default", "other-key"),
		}[s], nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []coreV1.EnvVar{
		{Name: "__ref_0960osB2KjQY08QKfHliCg", ValueFrom: &coreV1.EnvVarSource{
			SecretKeyRef: &coreV1.SecretKeySelector{
				LocalObjectReference: coreV1.LocalObjectReference{Name: "default"},
				Key:                  "some-key",
			},
		}},
		{Name: "__ref_mWObImRl7lfuP04NHDPsvA", ValueFrom: &coreV1.EnvVarSource{
			SecretKeyRef: &coreV1.SecretKeySelector{
				LocalObjectReference: coreV1.LocalObjectReference{Name: "default"},
				Key:                  "other-key",
			},
		}},
		{Name: "BUZZ", Value: "FIZZ"},
		{Name: "FIZZ", Value: "BUZZ"},
		{Name: "KEY", Value: "$(__ref_0960osB2KjQY08QKfHliCg) $(__ref_mWObImRl7lfuP04NHDPsvA)"},
	}, out)
}
