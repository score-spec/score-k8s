package internal

import "slices"

const (
	AnnotationPrefix       = "k8s.score.dev/"
	WorkloadKindAnnotation = AnnotationPrefix + "kind"
)

func ListAnnotations(metadata map[string]interface{}) []string {
	if a, ok := metadata["annotations"].(map[string]interface{}); ok {
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
	if a, ok := metadata["annotations"].(map[string]interface{}); ok {
		if v, ok := a[annotation].(string); ok {
			return v, true
		}
	}
	return "", false
}
