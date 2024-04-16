package internal

import (
	appsV1 "k8s.io/api/apps/v1"
	appsV1b1 "k8s.io/api/apps/v1beta1"
	appsV1b2 "k8s.io/api/apps/v1beta2"
	coreV1 "k8s.io/api/core/v1"
	networkingV1 "k8s.io/api/networking/v1"
	networkingV1b1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var K8sCodecFactory = serializer.CodecFactory{}

func init() {
	scheme := runtime.NewScheme()
	_ = coreV1.AddToScheme(scheme)
	_ = appsV1.AddToScheme(scheme)
	_ = appsV1b1.AddToScheme(scheme)
	_ = appsV1b2.AddToScheme(scheme)
	_ = networkingV1.AddToScheme(scheme)
	_ = networkingV1b1.AddToScheme(scheme)
	K8sCodecFactory = serializer.NewCodecFactory(scheme)
}
