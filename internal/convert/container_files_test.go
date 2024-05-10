package convert

import (
	"testing"

	scoretypes "github.com/score-spec/score-go/types"
	"github.com/stretchr/testify/assert"
	coreV1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/score-spec/score-k8s/internal"
)

func Test_convertContainerFile_invalid_mode(t *testing.T) {
	_, _, _, err := convertContainerFile(0, scoretypes.ContainerFilesElem{Mode: internal.Ref("xxx")}, "", nil, nil)
	assert.EqualError(t, err, "mode: failed to parse 'xxx': strconv.ParseInt: parsing \"xxx\": invalid syntax")
}

func Test_convertContainerFile_no_content(t *testing.T) {
	_, _, _, err := convertContainerFile(0, scoretypes.ContainerFilesElem{}, "", nil, nil)
	assert.EqualError(t, err, "missing 'content' or 'source'")
}

func Test_convertContainerFile_unreadable_source(t *testing.T) {
	_, _, _, err := convertContainerFile(0, scoretypes.ContainerFilesElem{Source: internal.Ref("file.that.does.not.exist")}, "", nil, nil)
	assert.EqualError(t, err, "source: failed to read file 'file.that.does.not.exist': open file.that.does.not.exist: no such file or directory")
}

func Test_convertContainerFile_unreadable_source_relative(t *testing.T) {
	_, _, _, err := convertContainerFile(0, scoretypes.ContainerFilesElem{Source: internal.Ref("file.that.does.not.exist")}, "", internal.Ref("my/file.yaml"), nil)
	assert.EqualError(t, err, "source: failed to read file 'my/file.that.does.not.exist': open my/file.that.does.not.exist: no such file or directory")
}

func Test_convertContainerFile_content_no_expand(t *testing.T) {
	mount, cfg, vol, err := convertContainerFile(0, scoretypes.ContainerFilesElem{
		Content:  internal.Ref("raw content with ${some.ref}"),
		Target:   "/some/mount",
		NoExpand: internal.Ref(true),
	}, "my-workload-c1-", nil, nil)
	assert.Equal(t, coreV1.VolumeMount{
		Name:      "file-0",
		MountPath: "/some",
	}, mount)
	if assert.NotNil(t, cfg) {
		assert.Equal(t, coreV1.ConfigMap{
			TypeMeta:   v1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: v1.ObjectMeta{Name: "my-workload-c1-file-0"},
			BinaryData: map[string][]byte{
				"file": []byte("raw content with ${some.ref}"),
			},
		}, *cfg)
	}
	if assert.NotNil(t, vol) {
		assert.Equal(t, coreV1.Volume{
			Name: "file-0",
			VolumeSource: coreV1.VolumeSource{
				ConfigMap: &coreV1.ConfigMapVolumeSource{
					LocalObjectReference: coreV1.LocalObjectReference{Name: "my-workload-c1-file-0"},
					Items: []coreV1.KeyToPath{
						{Key: "file", Path: "mount"},
					},
				},
			},
		}, *vol)
	}
	assert.NoError(t, err)
}

func Test_convertContainerFile_content_expand_mixed(t *testing.T) {
	_, _, _, err := convertContainerFile(0, scoretypes.ContainerFilesElem{
		Content: internal.Ref("raw content with ${some.ref}"),
		Target:  "/some/mount",
	}, "my-workload-c1-", nil, func(s string) (string, error) {
		return internal.EncodeSecretReference("default", "key"), nil
	})
	assert.EqualError(t, err, "content: contained a mix of secret references and raw content")
}

func Test_convertContainerFile_content_expand_with_secret(t *testing.T) {
	mount, cfg, vol, err := convertContainerFile(0, scoretypes.ContainerFilesElem{
		Content: internal.Ref("${some.ref}"),
		Target:  "/some/mount",
	}, "my-workload-c1-", nil, func(s string) (string, error) {
		return internal.EncodeSecretReference("default", "key"), nil
	})
	assert.Equal(t, coreV1.VolumeMount{
		Name:      "file-0",
		MountPath: "/some",
	}, mount)
	assert.Nil(t, cfg)
	if assert.NotNil(t, vol) {
		assert.Equal(t, coreV1.Volume{
			Name: "file-0",
			VolumeSource: coreV1.VolumeSource{
				Secret: &coreV1.SecretVolumeSource{
					SecretName: "default",
					Items: []coreV1.KeyToPath{
						{Key: "key", Path: "mount"},
					},
				},
			},
		}, *vol)
	}
	assert.NoError(t, err)
}
