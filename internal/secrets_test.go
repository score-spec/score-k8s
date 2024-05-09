package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeSecretReferences_nominal(t *testing.T) {
	splits, refs, err := DecodeSecretReferences(
		EncodeSecretReference("s1", "k1") + "thing" +
			EncodeSecretReference("s2", "k2") +
			EncodeSecretReference("s3", "k3"),
	)
	assert.NoError(t, err)
	assert.Equal(t, []string{"", "thing", "", ""}, splits)
	assert.Equal(t, []SecretRef{
		{Name: "s1", Key: "k1"},
		{Name: "s2", Key: "k2"},
		{Name: "s3", Key: "k3"},
	}, refs)
}
