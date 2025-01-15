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
	"slices"
)

const (
	AnnotationPrefix              = "k8s.score.dev/"
	WorkloadKindAnnotation        = AnnotationPrefix + "kind"
	WorkloadServiceNameAnnotation = AnnotationPrefix + "service-name"
)

func GetAnnotations(metadata map[string]interface{}) map[string]interface{} {
	a, _ := metadata["annotations"].(map[string]interface{})
	return a
}

func ListAnnotations(metadata map[string]interface{}) []string {
	if a := GetAnnotations(metadata); a != nil {
		out := make([]string, 0, len(a))
		for s, _ := range a {
			out = append(out, s)
		}
		slices.Sort(out)
		return out
	}
	return nil
}

func FindAnnotation(metadata map[string]interface{}, annotation string) (string, bool) {
	if a := GetAnnotations(metadata); a != nil {
		if v, ok := a[annotation].(string); ok {
			return v, true
		}
	}
	return "", false
}
