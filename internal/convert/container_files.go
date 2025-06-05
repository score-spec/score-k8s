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
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"crypto/sha256"
	"unicode/utf8"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	coreV1 "k8s.io/api/core/v1"
	machineryMeta "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/score-spec/score-k8s/internal"
)

func convertContainerFile(
    target string, file scoretypes.ContainerFile,
	manifestPrefix string, scoreSpecPath *string, substitutionFunc func(string) (string, error),
) (coreV1.VolumeMount, *coreV1.ConfigMap, *coreV1.Volume, error) {
	targetHash := sha256.Sum256([]byte(target))
	mount := coreV1.VolumeMount{
		Name:      fmt.Sprintf("file-%x", targetHash[:5]),
		ReadOnly:  false,
		MountPath: filepath.Dir(target),
	}

	var mountMode *int32
	if file.Mode != nil {
		df, err := strconv.ParseInt(*file.Mode, 8, 32)
		if err != nil {
			return mount, nil, nil, errors.Wrapf(err, "mode: failed to parse '%s'", *file.Mode)
		}
		mountMode = internal.Ref(int32(df))
	}

	var content []byte
	var err error
	if file.Content != nil {
		content = []byte(*file.Content)
	} else if file.BinaryContent != nil {
		content, err = base64.StdEncoding.DecodeString(*file.BinaryContent)
		if err != nil {
			return mount, nil, nil, fmt.Errorf("binaryContent: failed to decode base64: %w", err)
		}
	} else if file.Source != nil {
		sourcePath := *file.Source
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

	if (file.NoExpand == nil || !*file.NoExpand) && file.BinaryContent == nil {
		if !utf8.Valid(content) {
			return mount, nil, nil, errors.New("source content contains non-utf8 bytes; set noExpand=true or use binaryContent")
		}
		stringContent, err := framework.SubstituteString(string(content), substitutionFunc)
		if err != nil {
			return mount, nil, nil, errors.Wrap(err, "failed to substitute in content")
		}

		parts, refs, err := internal.DecodeSecretReferences(stringContent)
		if err != nil {
			return mount, nil, nil, errors.Wrap(err, "content: failed to resolve secret")
		}

		if len(refs) > 0 {
			// If the file content is made up of a reference to just a secret, we can allow that
			if len(refs) == 1 && parts[0] == "" && parts[1] == "" {
				return mount, nil, &coreV1.Volume{
					Name: mount.Name,
					VolumeSource: coreV1.VolumeSource{
						Secret: &coreV1.SecretVolumeSource{
							SecretName: refs[0].Name,
							Items: []coreV1.KeyToPath{{
								Key:  refs[0].Key,
								Path: filepath.Base(target),
								Mode: mountMode}},
						},
					},
				}, nil
			}
			// Anything else is invalid
			return mount, nil, nil, errors.New("content: contained a mix of secret references and raw content")
		}

		content = []byte(stringContent)
	}

	configMapName := fmt.Sprintf("%sfile-%x", manifestPrefix, targetHash[:5])
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
						Path: filepath.Base(target),
						Mode: mountMode,
					}},
					LocalObjectReference: coreV1.LocalObjectReference{Name: configMapName},
				},
			},
		},
		nil
}
