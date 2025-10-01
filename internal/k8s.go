// Copyright 2024 The Score Authors
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
var YamlSerializerInfo = runtime.SerializerInfo{}

func init() {
	scheme := runtime.NewScheme()
	_ = coreV1.AddToScheme(scheme)
	_ = appsV1.AddToScheme(scheme)
	_ = appsV1b1.AddToScheme(scheme)
	_ = appsV1b2.AddToScheme(scheme)
	_ = networkingV1.AddToScheme(scheme)
	_ = networkingV1b1.AddToScheme(scheme)
	K8sCodecFactory = serializer.NewCodecFactory(scheme)
	YamlSerializerInfo, _ = runtime.SerializerInfoForMediaType(K8sCodecFactory.SupportedMediaTypes(), runtime.ContentTypeYAML)
}
