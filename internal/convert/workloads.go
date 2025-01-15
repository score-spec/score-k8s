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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	v1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	machineryMeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/project"
)

const (
	WorkloadKindDeployment  = "Deployment"
	WorkloadKindStatefulSet = "StatefulSet"

	SelectorLabelName      = "app.kubernetes.io/name"
	SelectorLabelInstance  = "app.kubernetes.io/instance"
	SelectorLabelManagedBy = "app.kubernetes.io/managed-by"
)

func ConvertWorkload(state *project.State, workloadName string) ([]machineryMeta.Object, error) {
	resOutputs, err := state.GetResourceOutputForWorkload(workloadName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate outputs")
	}
	sf := framework.BuildSubstitutionFunction(state.Workloads[workloadName].Spec.Metadata, resOutputs)

	spec := state.Workloads[workloadName].Spec
	manifests := make([]machineryMeta.Object, 0, 1)

	// containers and volumes here are fun..
	// we have to collect them all based on the parent paths they get mounted in and turn these into projected volumes
	// then add the projected volumes to the deployment
	volumes := make([]coreV1.Volume, 0)
	volumeClaimTemplates := make([]coreV1.PersistentVolumeClaim, 0)

	containers := make([]coreV1.Container, 0, len(spec.Containers))
	containerNames := make([]string, 0, len(spec.Containers))
	for name := range spec.Containers {
		containerNames = append(containerNames, name)
	}
	slices.Sort(containerNames)

	commonLabels := map[string]string{
		SelectorLabelName:      workloadName,
		SelectorLabelInstance:  workloadName + state.Workloads[workloadName].Extras.InstanceSuffix,
		SelectorLabelManagedBy: "score-k8s",
	}

	for _, containerName := range containerNames {
		container := spec.Containers[containerName]
		c := coreV1.Container{
			Name:         containerName,
			Image:        container.Image,
			Command:      container.Command,
			Args:         container.Args,
			VolumeMounts: make([]coreV1.VolumeMount, 0),
		}

		c.Resources, err = convertContainerResources(container.Resources)
		if err != nil {
			return nil, errors.Wrapf(err, "containers.%s.resources: failed to convert", containerName)
		}

		c.Env, err = convertContainerVariables(container.Variables, sf)
		if err != nil {
			return nil, errors.Wrapf(err, "containers.%s.variables: failed to convert", containerName)
		}

		containerVolumes := make([]coreV1.Volume, 0)
		containerVolumeMounts := make([]coreV1.VolumeMount, 0)

		volSubstitutionFunction := func(ref string) (string, error) {
			if parts := framework.SplitRefParts(ref); len(parts) == 2 && parts[0] == "resources" {
				resName := parts[1]
				if res, ok := spec.Resources[resName]; ok {
					return string(framework.NewResourceUid(workloadName, resName, res.Type, res.Class, res.Id)), nil
				}
				return "", fmt.Errorf("resource '%s' does not exist", resName)
			}
			return sf(ref)
		}
		for i, volume := range container.Volumes {
			if mount, vol, claim, err := convertContainerVolume(i, volume, state.Resources, volSubstitutionFunction); err != nil {
				return nil, errors.Wrapf(err, "containers.%s.volumes.%d: failed to convert", containerName, i)
			} else {
				containerVolumeMounts = append(containerVolumeMounts, mount)
				if claim != nil {
					volumeClaimTemplates = append(volumeClaimTemplates, *claim)
				} else if vol != nil {
					containerVolumes = append(containerVolumes, *vol)
				}
			}
		}

		for i, f := range container.Files {
			if mount, cfg, vol, err := convertContainerFile(i, f, fmt.Sprintf("%s-%s-", workloadName, containerName), state.Workloads[workloadName].File, sf); err != nil {
				return nil, errors.Wrapf(err, "containers.%s.files.%d: failed to convert", containerName, i)
			} else {
				containerVolumeMounts = append(containerVolumeMounts, mount)
				if cfg != nil {
					manifests = append(manifests, cfg)
				}
				if vol != nil {
					containerVolumes = append(containerVolumes, *vol)
				}
			}
		}

		// collapse projected volume mounts
		containerVolumes, containerVolumeMounts, err = collapseVolumeMounts(containerVolumes, containerVolumeMounts)
		if err != nil {
			return nil, errors.Wrapf(err, "containers.%s.volumes: failed to combine projected volumes", containerName)
		}
		c.VolumeMounts = containerVolumeMounts
		volumes = append(volumes, containerVolumes...)

		if container.LivenessProbe != nil {
			c.LivenessProbe = &coreV1.Probe{ProbeHandler: buildProbe(container.LivenessProbe.HttpGet)}
		}
		if container.ReadinessProbe != nil {
			c.ReadinessProbe = &coreV1.Probe{ProbeHandler: buildProbe(container.ReadinessProbe.HttpGet)}
		}

		containers = append(containers, c)
	}

	portList := make([]coreV1.ServicePort, 0)
	if spec.Service != nil && len(spec.Service.Ports) > 0 {
		for portName, port := range spec.Service.Ports {
			var proto = coreV1.ProtocolTCP
			if port.Protocol != nil && *port.Protocol != "" {
				proto = coreV1.Protocol(strings.ToUpper(string(*port.Protocol)))
			}
			var targetPort = port.Port
			if port.TargetPort != nil && *port.TargetPort > 0 {
				targetPort = *port.TargetPort // Defaults to the published port
			}
			portList = append(portList, coreV1.ServicePort{
				Name:       portName,
				Port:       int32(port.Port),
				TargetPort: intstr.FromInt32(int32(targetPort)),
				Protocol:   proto,
			})
		}
	}

	converterInputs := ConverterInputs{
		WorkloadName:        workloadName,
		WorkloadAnnotations: internal.GetAnnotations(spec.Metadata),
		PodTemplate: coreV1.PodTemplateSpec{
			ObjectMeta: machineryMeta.ObjectMeta{
				Labels: commonLabels,
				// We want to apply the annotations from the workload onto the pod.
				// See the doc of buildPodAnnotations for what gets included here.
				Annotations: buildPodAnnotations(spec.Metadata),
			},
			Spec: coreV1.PodSpec{Containers: containers, Volumes: volumes},
		},
		VolumeClaimTemplates: volumeClaimTemplates,
		ServicePorts:         portList,
	}

	// This might seem silly, but really we're doing a round trip of the json conversion to ensure we don't cheat in our internal version
	rawConverterInputs, err := json.Marshal(converterInputs)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize converter inputs")
	}
	var rawManifests []byte
	if cb := state.Workloads[workloadName].Extras.ConverterBinary; cb != "" {
		parts := strings.Split(cb, ",")
		c := exec.Command(parts[0], parts...)
		c.Env = os.Environ()
		c.Stdin = bytes.NewReader(rawConverterInputs)
		c.Stderr = os.Stderr
		buf := new(bytes.Buffer)
		c.Stdout = buf
		if err := c.Run(); err != nil {
			return nil, fmt.Errorf("failed to run converter binary '%s' on inputs: %w", cb, err)
		}
		rawManifests = buf.Bytes()
	} else {
		rawManifests, err = ConvertRawInputsToRawManifests(rawConverterInputs)
		if err != nil {
			return nil, err
		}
		var convertedManifests []machineryMeta.Object
		if err := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(rawManifests), 100).Decode(&convertedManifests); err != nil {
			return nil, fmt.Errorf("failed to decode convert outputs into manifests: %w", err)
		}
		manifests = append(manifests, convertedManifests...)
	}
	return manifests, nil
}

func ConvertRawInputsToRawManifests(rawInputs []byte) ([]byte, error) {
	var inputs ConverterInputs
	if err := json.Unmarshal(rawInputs, &inputs); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	} else if manifests, err := ConvertInputsToManifest(inputs); err != nil {
		return nil, fmt.Errorf("failed to convert: %w", err)
	} else {
		return json.Marshal(manifests)
	}
}

func ConvertInputsToManifest(inputs ConverterInputs) ([]machineryMeta.Object, error) {
	manifests := make([]machineryMeta.Object, 0)

	kind := WorkloadKindDeployment
	if d, ok := inputs.WorkloadAnnotations[internal.WorkloadKindAnnotation].(string); ok && d != "" {
		kind = d
		if kind != WorkloadKindDeployment && kind != WorkloadKindStatefulSet {
			return nil, fmt.Errorf("metadata: annotations: %s: unsupported workload kind", internal.WorkloadKindAnnotation)
		}
	}

	topLevelAnnotations := map[string]string{
		internal.AnnotationPrefix + "workload-name": inputs.WorkloadName,
	}

	switch kind {
	case WorkloadKindDeployment:
		manifests = append(manifests, &v1.Deployment{
			TypeMeta: machineryMeta.TypeMeta{Kind: WorkloadKindDeployment, APIVersion: "apps/v1"},
			ObjectMeta: machineryMeta.ObjectMeta{
				Name:        inputs.WorkloadName,
				Annotations: topLevelAnnotations,
				Labels:      inputs.PodTemplate.Labels,
			},
			Spec: v1.DeploymentSpec{
				Selector: &machineryMeta.LabelSelector{
					MatchLabels: map[string]string{
						SelectorLabelInstance: inputs.PodTemplate.Labels[SelectorLabelInstance],
					},
				},
				Template: inputs.PodTemplate,
			},
		})
	case WorkloadKindStatefulSet:

		// need to allocate a headless service here
		headlessServiceName := fmt.Sprintf("%s-headless-svc", inputs.WorkloadName)
		manifests = append(manifests, &coreV1.Service{
			TypeMeta: machineryMeta.TypeMeta{Kind: "Service", APIVersion: "v1"},
			ObjectMeta: machineryMeta.ObjectMeta{
				Name:        headlessServiceName,
				Annotations: topLevelAnnotations,
				Labels:      inputs.PodTemplate.Labels,
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{
					SelectorLabelInstance: inputs.PodTemplate.Labels[SelectorLabelInstance],
				},
				ClusterIP: "None",
				Ports:     []coreV1.ServicePort{{Name: "default", Port: 99, TargetPort: intstr.FromInt32(99)}},
			},
		})

		manifests = append(manifests, &v1.StatefulSet{
			TypeMeta: machineryMeta.TypeMeta{Kind: WorkloadKindStatefulSet, APIVersion: "apps/v1"},
			ObjectMeta: machineryMeta.ObjectMeta{
				Name:        inputs.WorkloadName,
				Annotations: topLevelAnnotations,
				Labels:      inputs.PodTemplate.Labels,
			},
			Spec: v1.StatefulSetSpec{
				Selector: &machineryMeta.LabelSelector{
					MatchLabels: map[string]string{
						SelectorLabelInstance: inputs.PodTemplate.Labels[SelectorLabelInstance],
					},
				},
				ServiceName: headlessServiceName,
				Template:    inputs.PodTemplate,
				// So the puzzle here is how to get this from our volumes...
				VolumeClaimTemplates: inputs.VolumeClaimTemplates,
			},
		})
	}

	return manifests, nil
}

func WorkloadServiceName(workloadName string, specMetadata map[string]interface{}) string {
	if d, ok := internal.FindAnnotation(specMetadata, internal.WorkloadServiceNameAnnotation); ok {
		return d
	}
	return workloadName
}

func buildProbe(input scoretypes.HttpProbe) coreV1.ProbeHandler {
	ph := coreV1.ProbeHandler{
		HTTPGet: &coreV1.HTTPGetAction{
			Path:   input.Path,
			Port:   intstr.FromInt32(int32(input.Port)),
			Host:   internal.DerefOr(input.Host, ""),
			Scheme: coreV1.URIScheme(internal.DerefOr(input.Scheme, "")),
		},
	}
	if len(input.HttpHeaders) > 0 {
		h := make([]coreV1.HTTPHeader, 0, len(input.HttpHeaders))
		for _, header := range input.HttpHeaders {
			h = append(h, coreV1.HTTPHeader{Name: header.Name, Value: header.Value})
		}
		ph.HTTPGet.HTTPHeaders = h
	}
	return ph
}

// buildPodAnnotations builds the annotations map for a pod by copying the workload annotations
// and removing any annotations scoped score-k8s.
func buildPodAnnotations(metadata map[string]interface{}) map[string]string {
	out := make(map[string]string)
	for _, s := range internal.ListAnnotations(metadata) {
		if v, ok := internal.FindAnnotation(metadata, s); ok && !strings.HasPrefix(s, internal.AnnotationPrefix) {
			out[s] = v
		}
	}
	out[internal.AnnotationPrefix+"workload-name"] = metadata["name"].(string)
	return out
}

type ConverterInputs struct {
	WorkloadName         string                         `json:"workload_name"`
	PodTemplate          coreV1.PodTemplateSpec         `json:"pod_template"`
	VolumeClaimTemplates []coreV1.PersistentVolumeClaim `json:"volume_claim_templates"`
	ServicePorts         []coreV1.ServicePort           `json:"service_ports"`
	WorkloadAnnotations  map[string]interface{}         `json:"workload_annotations"`
}
