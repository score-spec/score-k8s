package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeSecretReferences_nominal(t *testing.T) {
	splits, refs, err := DecodeSecretReferences(
		EncodeSecretReference("ns", "s1", "k1") + "thing" +
			EncodeSecretReference("ns", "s2", "k2") +
			EncodeSecretReference("ns", "s3", "k3"),
	)
	assert.NoError(t, err)
	assert.Equal(t, []string{"", "thing", "", ""}, splits)
	assert.Equal(t, []SecretRef{
		{Namespace: "ns", Name: "s1", Key: "k1"},
		{Namespace: "ns", Name: "s2", Key: "k2"},
		{Namespace: "ns", Name: "s3", Key: "k3"},
	}, refs)
}
