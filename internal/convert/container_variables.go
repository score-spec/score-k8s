package convert

import (
	"encoding/base64"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	coreV1 "k8s.io/api/core/v1"

	"github.com/score-spec/score-k8s/internal"
)

func generateSecretRefEnvVarName(secretName, key string) string {
	h := fnv.New128()
	_, _ = h.Write([]byte(secretName))
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("__ref_%s", strings.NewReplacer("_", "0", "-", "0").Replace(base64.RawURLEncoding.EncodeToString(h.Sum(nil))))
}

func convertContainerVariable(key, value string, substitutionFunction func(string) (string, error)) ([]coreV1.EnvVar, error) {
	resolvedValue, err := framework.SubstituteString(value, substitutionFunction)
	if err != nil {
		return nil, errors.Wrap(err, "failed to substitute placeholders")
	}

	parts, refs, err := internal.DecodeSecretReferences(resolvedValue)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve secret references")
	}

	// No secret references - return the resolves value
	if len(refs) == 0 {
		return []coreV1.EnvVar{{Name: key, Value: resolvedValue}}, nil
	}

	// One secret reference taking up the whole value
	if len(refs) == 1 && parts[0] == "" && parts[1] == "" {
		return []coreV1.EnvVar{{Name: key, ValueFrom: &coreV1.EnvVarSource{
			SecretKeyRef: &coreV1.SecretKeySelector{
				LocalObjectReference: coreV1.LocalObjectReference{Name: refs[0].Name},
				Key:                  refs[0].Key,
			},
		}}}, nil
	}

	// One or more secret references with a mix of other content
	out := make([]coreV1.EnvVar, 0, 1+len(refs))

	// First build the referenced secrets
	for _, ref := range refs {
		out = append(out, coreV1.EnvVar{
			Name: generateSecretRefEnvVarName(ref.Name, ref.Key),
			ValueFrom: &coreV1.EnvVarSource{
				SecretKeyRef: &coreV1.SecretKeySelector{
					LocalObjectReference: coreV1.LocalObjectReference{Name: ref.Name},
					Key:                  ref.Key,
				},
			},
		})
	}

	// Then build the final env var string.
	sb := new(strings.Builder)
	for i, part := range parts {
		if i > 0 {
			sb.WriteString(fmt.Sprintf("$(%s)", generateSecretRefEnvVarName(refs[i-1].Name, refs[i-1].Key)))
		}
		sb.WriteString(part)
	}
	out = append(out, coreV1.EnvVar{Name: key, Value: sb.String()})

	return out, nil
}

func convertContainerVariables(variables scoretypes.ContainerVariables, substitutionFunction func(string) (string, error)) ([]coreV1.EnvVar, error) {
	out := make([]coreV1.EnvVar, 0, len(variables))
	for k, v := range variables {
		adds, err := convertContainerVariable(k, v, substitutionFunction)
		if err != nil {
			return nil, errors.Wrapf(err, "'%s': failed to convert", k)
		}
		// don't add things twice - check for duplicates
		for _, add := range adds {
			if !slices.ContainsFunc(out, func(envVar coreV1.EnvVar) bool {
				return envVar.Name == add.Name
			}) {
				out = append(out, add)
			}
		}
	}
	slices.SortFunc(out, func(a, b coreV1.EnvVar) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}
