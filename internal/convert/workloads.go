package convert

import (
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
				v2, err := sf(v)
				if err != nil {
					return nil, errors.Wrapf(err, "variables.%s: failed to substitute placeholders", k)
				}
				ev = append(ev, coreV1.EnvVar{Name: k, Value: v2, ValueFrom: &coreV1.EnvVarSource{}})
			}
			c.Env = ev
		}

		if len(container.Volumes) > 0 {
			// TODO: volumes
			return nil, errors.New("volumes not supported")
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

	// TODO: collapse volumes together with projected volumes

	// TODO: support annotations to turn this into a cronjob

	manifests = append(manifests, &v1.Deployment{
		TypeMeta: machineryMeta.TypeMeta{Kind: "Deployment", APIVersion: "apps/v1"},
		ObjectMeta: machineryMeta.ObjectMeta{
			Name: workloadName,
			Labels: map[string]string{
				"app": workloadName,
			},
		},
		Spec: v1.DeploymentSpec{
			Replicas: internal.Ref(int32(1)),
			Selector: &machineryMeta.LabelSelector{
				MatchExpressions: []machineryMeta.LabelSelectorRequirement{{"app", machineryMeta.LabelSelectorOpIn, []string{workloadName}}},
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
