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

func Test_convertContainerVariable_0(t *testing.T) {
	out, err := convertContainerVariable("KEY", "VALUE", nil)
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
