package internal

const (
	annotationPrefix       = "k8s.score.dev/"
	WorkloadKindAnnotation = annotationPrefix + "kind"
)

func FindAnnotation[K any](metadata map[string]interface{}, annotation string) (K, bool) {
	var zero K
	if a, ok := metadata["annotations"].(map[string]interface{}); ok {
		if v, ok := a[annotation].(K); ok {
			return v, true
		}
	}
	return zero, false
}
