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

package convert

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	coreV1 "k8s.io/api/core/v1"
	machineryMeta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/score-spec/score-k8s/internal"
)

func convertContainerFile(
	index int, fileElem scoretypes.ContainerFilesElem,
	manifestPrefix string, scoreSpecPath *string, substitutionFunc func(string) (string, error),
) (coreV1.VolumeMount, *coreV1.ConfigMap, *coreV1.Volume, error) {
	mount := coreV1.VolumeMount{
		Name:      fmt.Sprintf("file-%d", index),
		ReadOnly:  false,
		MountPath: filepath.Dir(fileElem.Target),
	}

	var mountMode *int32
	if fileElem.Mode != nil {
		df, err := strconv.ParseInt(*fileElem.Mode, 8, 32)
		if err != nil {
			return mount, nil, nil, errors.Wrapf(err, "mode: failed to parse '%s'", *fileElem.Mode)
		}
		mountMode = internal.Ref(int32(df))
	}

	var content []byte
	var err error
	if fileElem.Content != nil {
		content = []byte(*fileElem.Content)
	} else if fileElem.Source != nil {
		sourcePath := *fileElem.Source
		if !filepath.IsAbs(sourcePath) && scoreSpecPath != nil {
			sourcePath = filepath.Join(filepath.Dir(*scoreSpecPath), sourcePath)
		}
		content, err = os.ReadFile(sourcePath)
		if err != nil {
			return mount, nil, nil, errors.Wrapf(err, "source: failed to read file '%s'", sourcePath)
		}
	} else {
		return mount, nil, nil, errors.New("missing 'content' or 'source'")
	}

	if fileElem.NoExpand == nil || !*fileElem.NoExpand {
		stringContent, err := framework.SubstituteString(string(content), substitutionFunc)
		if err != nil {
			return mount, nil, nil, errors.Wrap(err, "failed to substitute in content")
		}
		content = []byte(stringContent)
	}

	parts, refs, err := internal.DecodeSecretReferences(string(content))
	if err != nil {
		return mount, nil, nil, errors.Wrap(err, "content: failed to resolve secret")
	}

	// No secret refs means this is plain content that we can write directly as a config map.
	if len(refs) == 0 {
		configMapName := fmt.Sprintf("%sfile-%d", manifestPrefix, index)
		return mount,
			&coreV1.ConfigMap{
				TypeMeta: machineryMeta.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
				ObjectMeta: machineryMeta.ObjectMeta{
					Name: configMapName,
				},
				BinaryData: map[string][]byte{"file": content},
			}, &coreV1.Volume{
				Name: mount.Name,
				VolumeSource: coreV1.VolumeSource{
					ConfigMap: &coreV1.ConfigMapVolumeSource{
						Items: []coreV1.KeyToPath{{
							Key:  "file",
							Path: filepath.Base(fileElem.Target),
							Mode: mountMode,
						}},
						LocalObjectReference: coreV1.LocalObjectReference{Name: configMapName},
					},
				},
			},
			nil
	}

	// If the file content is made up of a reference to just a secret, we can allow that
	if len(refs) == 1 && parts[0] == "" && parts[1] == "" {
		return mount, nil, &coreV1.Volume{
			Name: mount.Name,
			VolumeSource: coreV1.VolumeSource{
				Secret: &coreV1.SecretVolumeSource{
					SecretName: refs[0].Name,
					Items: []coreV1.KeyToPath{{
						Key:  refs[0].Key,
						Path: filepath.Base(fileElem.Target),
						Mode: mountMode}},
				},
			},
		}, nil
	}

	// Anything else is invalid
	return mount, nil, nil, errors.New("content: contained a mix of secret references and raw content")
}
