package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/score-spec/score-go/framework"
	scoretypes "github.com/score-spec/score-go/types"
	v1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	machineryMeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/project"
)

const (
	WorkloadKindDeployment  = "Deployment"
	WorkloadKindStatefulSet = "StatefulSet"
)

func ConvertWorkload(state *project.State, workloadName string) ([]machineryMeta.Object, error) {
	resOutputs, err := state.GetResourceOutputForWorkload(workloadName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate outputs")
	}
	sf := framework.BuildSubstitutionFunction(state.Workloads[workloadName].Spec.Metadata, resOutputs)

	spec := state.Workloads[workloadName].Spec
	manifests := make([]machineryMeta.Object, 0, 1)

	kind := WorkloadKindDeployment
	if d, ok := internal.FindAnnotation[string](spec.Metadata, internal.WorkloadKindAnnotation); ok {
		kind = d
		if kind != WorkloadKindDeployment && kind != WorkloadKindStatefulSet {
			return nil, errors.Wrapf(err, "metadata: annotations: %s: unsupported workload kind", internal.WorkloadKindAnnotation)
		}
	}

	// containers and volumes here are fun..
	// we have to collect them all based on the parent paths they get mounted in and turn these into projected volumes
	// then add the projected volumes to the deployment
	volumes := make([]coreV1.Volume, 0)
	volumeClaimTemplates := make([]coreV1.PersistentVolumeClaim, 0)

	containers := make([]coreV1.Container, 0, len(spec.Containers))
	for containerName, container := range spec.Containers {
		c := coreV1.Container{
			Name:         containerName,
			Image:        container.Image,
			Command:      container.Command,
			Args:         container.Args,
			VolumeMounts: make([]coreV1.VolumeMount, 0),
		}

		if container.Resources != nil {
			if container.Resources.Requests != nil {
				c.Resources.Requests, err = buildResourceList(container.Resources.Requests)
				if err != nil {
					return nil, errors.Wrapf(err, "containers.%s.resources.requests", containerName)
				}
				c.Resources.Limits, err = buildResourceList(container.Resources.Limits)
				if err != nil {
					return nil, errors.Wrapf(err, "containers.%s.resources.limits", containerName)
				}
			}
		}

		if len(container.Variables) > 0 {
			ev := make([]coreV1.EnvVar, 0, len(container.Variables))
			for k, v := range container.Variables {
				v2, err := framework.SubstituteString(v, sf)
				if err != nil {
					return nil, errors.Wrapf(err, "containers.%s.variables.%s: failed to substitute placeholders", containerName, k)
				}
				ev = append(ev, coreV1.EnvVar{Name: k, Value: v2, ValueFrom: &coreV1.EnvVarSource{}})
			}
			c.Env = ev

			// TODO: resolve secret env vars here
		}

		if len(container.Volumes) > 0 {
			for i, v := range container.Volumes {
				res, ok := state.Resources[framework.ResourceUid(v.Source)]
				if !ok {
					return nil, errors.Wrapf(err, "containers.%s.volumes.%d: resource '%s' does not exist", containerName, i, v.Source)
				}
				volName := fmt.Sprintf("vol-%d", i)

				// convert the outputs into a spec
				raw, _ := json.Marshal(res.Outputs)
				var anon struct {
					Source    *coreV1.VolumeSource              `json:"source"`
					ClaimSpec *coreV1.PersistentVolumeClaimSpec `json:"claimSpec"`
				}
				dec := json.NewDecoder(bytes.NewReader(raw))
				dec.DisallowUnknownFields()
				if err = dec.Decode(&anon); err != nil {
					return nil, errors.Wrapf(err, "containers.%s.volumes.%d: failed to convert resource '%s' outputs into volume", containerName, i, v.Source)
				}
				if (anon.ClaimSpec == nil) == (anon.Source == nil) {
					return nil, errors.Errorf("containers.%s.volumes.%d: failed to convert resource '%s' outputs into volume: either 'source' or 'claimSpec' required", containerName, i, v.Source)
				} else if anon.ClaimSpec != nil && kind == WorkloadKindStatefulSet {
					volumeClaimTemplates = append(volumeClaimTemplates, coreV1.PersistentVolumeClaim{
						ObjectMeta: machineryMeta.ObjectMeta{
							Name: volName,
						},
						Spec: *anon.ClaimSpec,
					})
				} else if anon.Source != nil {
					volumes = append(volumes, coreV1.Volume{
						Name:         volName,
						VolumeSource: *anon.Source,
					})
				} else {
					return nil, errors.Errorf("containers.%s.volumes.%d: failed to convert resource '%s' outputs into volume: 'claimSpec' can only be used when workload is a statefulset", containerName, i, v.Source)
				}

				c.VolumeMounts = append(c.VolumeMounts, coreV1.VolumeMount{
					Name:      volName,
					MountPath: v.Target,
					SubPath:   internal.DerefOr(v.Path, ""),
					ReadOnly:  internal.DerefOr(v.ReadOnly, false),
				})
			}
		}

		if len(container.Files) > 0 {
			for i, f := range container.Files {
				var content []byte
				if f.Content != nil {
					content = []byte(*f.Content)
				} else if f.Source != nil {
					sourcePath := *f.Source
					if !filepath.IsAbs(sourcePath) && state.Workloads[workloadName].File != nil {
						sourcePath = filepath.Join(filepath.Dir(*state.Workloads[workloadName].File), sourcePath)
					}
					content, err = os.ReadFile(sourcePath)
					if err != nil {
						return nil, fmt.Errorf("containers.%s.files[%d].source: failed to read: %w", containerName, i, err)
					}
				} else {
					return nil, fmt.Errorf("containers.%s.files[%d]: missing 'content' or 'source'", containerName, i)
				}

				// TODO: identify and convert secret reference here

				configMapName := fmt.Sprintf("file-%s-%d", workloadName, i)
				manifests = append(manifests, &coreV1.ConfigMap{
					TypeMeta: machineryMeta.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
					ObjectMeta: machineryMeta.ObjectMeta{
						Name: configMapName,
					},
					BinaryData: map[string][]byte{"file": content},
				})

				var mountMode *int32
				if f.Mode != nil {
					df, err := strconv.ParseInt(*f.Mode, 10, 32)
					if err != nil {
						return nil, fmt.Errorf("containers.%s.files[%d]: failed to parse mode", containerName, i)
					}
					mountMode = internal.Ref(int32(df))
				}

				volumes = append(volumes, coreV1.Volume{
					Name: fmt.Sprintf("file-%d", i),
					VolumeSource: coreV1.VolumeSource{
						ConfigMap: &coreV1.ConfigMapVolumeSource{
							Items:                []coreV1.KeyToPath{{"file", filepath.Base(f.Target), mountMode}},
							LocalObjectReference: coreV1.LocalObjectReference{Name: configMapName},
						},
					},
				})

				c.VolumeMounts = append(c.VolumeMounts, coreV1.VolumeMount{
					Name:      fmt.Sprintf("file-%d", i),
					ReadOnly:  false,
					MountPath: filepath.Dir(f.Target),
				})
			}
		}

		if container.LivenessProbe != nil {
			c.LivenessProbe = &coreV1.Probe{ProbeHandler: buildProbe(container.LivenessProbe.HttpGet)}
		}
		if container.ReadinessProbe != nil {
			c.ReadinessProbe = &coreV1.Probe{ProbeHandler: buildProbe(container.ReadinessProbe.HttpGet)}
		}

		containers = append(containers, c)
	}

	// TODO: collapse all volumes together with projected volumes

	switch kind {
	case WorkloadKindDeployment:
		manifests = append(manifests, &v1.Deployment{
			TypeMeta: machineryMeta.TypeMeta{Kind: WorkloadKindDeployment, APIVersion: "apps/v1"},
			ObjectMeta: machineryMeta.ObjectMeta{
				Name:        workloadName,
				Annotations: make(map[string]string),
			},
			Spec: v1.DeploymentSpec{
				Replicas: internal.Ref(int32(1)),
				Selector: &machineryMeta.LabelSelector{
					MatchExpressions: []machineryMeta.LabelSelectorRequirement{
						{"app", machineryMeta.LabelSelectorOpIn, []string{workloadName}},
					},
				},
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: machineryMeta.ObjectMeta{
						Labels: map[string]string{
							"app": workloadName,
						},
					},
					Spec: coreV1.PodSpec{
						Containers: containers,
						Volumes:    volumes,
					},
				},
			},
		})
	case WorkloadKindStatefulSet:

		// need to allocate a headless service here
		headlessServiceName := fmt.Sprintf("%s-headless-svc", workloadName)
		manifests = append(manifests, &coreV1.Service{
			TypeMeta: machineryMeta.TypeMeta{Kind: "Service", APIVersion: "v1"},
			ObjectMeta: machineryMeta.ObjectMeta{
				Name:        headlessServiceName,
				Annotations: make(map[string]string),
			},
			Spec: coreV1.ServiceSpec{
				Selector: map[string]string{
					"app": workloadName,
				},
				ClusterIP: "None",
				Ports:     []coreV1.ServicePort{{Name: "default", Port: 99, TargetPort: intstr.FromInt32(99)}},
			},
		})

		manifests = append(manifests, &v1.StatefulSet{
			TypeMeta: machineryMeta.TypeMeta{Kind: WorkloadKindStatefulSet, APIVersion: "apps/v1"},
			ObjectMeta: machineryMeta.ObjectMeta{
				Name:        workloadName,
				Annotations: make(map[string]string),
			},
			Spec: v1.StatefulSetSpec{
				Replicas: internal.Ref(int32(1)),
				Selector: &machineryMeta.LabelSelector{
					MatchExpressions: []machineryMeta.LabelSelectorRequirement{
						{"app", machineryMeta.LabelSelectorOpIn, []string{workloadName}},
					},
				},
				ServiceName: headlessServiceName,
				Template: coreV1.PodTemplateSpec{
					ObjectMeta: machineryMeta.ObjectMeta{
						Labels: map[string]string{
							"app": workloadName,
						},
					},
					Spec: coreV1.PodSpec{
						Containers: containers,
						Volumes:    volumes,
					},
				},
				// So the puzzle here is how to get this from our volumes...
				VolumeClaimTemplates: volumeClaimTemplates,
			},
		})
	}

	return manifests, nil
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

func buildResourceList(input *scoretypes.ResourcesLimits) (coreV1.ResourceList, error) {
	var err error
	output := make(coreV1.ResourceList)
	if input.Cpu != nil {
		output["cpu"], err = resource.ParseQuantity(*input.Cpu)
		if err != nil {
			return nil, errors.Wrapf(err, "cpu: failed to parse")
		}
	}
	if input.Memory != nil {
		output["memory"], err = resource.ParseQuantity(*input.Memory)
		if err != nil {
			return nil, errors.Wrapf(err, "memory: failed to parse")
		}
	}
	return output, nil
}
