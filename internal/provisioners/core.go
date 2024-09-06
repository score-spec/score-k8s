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

package provisioners

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"

	"github.com/score-spec/score-go/framework"
	score "github.com/score-spec/score-go/types"

	util "github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/convert"
	"github.com/score-spec/score-k8s/internal/project"
)

// Input is the set of thins passed to the provisioner implementation. It provides context, previous state, and shared
// state used by all resources.
type Input struct {
	// -- aspects from the resource declaration --

	// Guid is a random uuid generated the first time this resource is added to the project.
	ResourceGuid     string                 `json:"resource_guid"`
	ResourceUid      string                 `json:"resource_uid"`
	ResourceType     string                 `json:"resource_type"`
	ResourceClass    string                 `json:"resource_class"`
	ResourceId       string                 `json:"resource_id"`
	ResourceParams   map[string]interface{} `json:"resource_params"`
	ResourceMetadata map[string]interface{} `json:"resource_metadata"`

	// -- aspects of the workloads --

	// SourceWorkload is the name of the workload that first defined this resource or carries the params definition.
	SourceWorkload string `json:"source_workload"`
	// WorkloadServices is a map from workload name to the network NetworkService of another workload which defines
	// the hostname and the set of ports it exposes.
	WorkloadServices map[string]NetworkService `json:"workload_services"`

	// -- current state --

	ResourceState map[string]interface{} `json:"resource_state"`
	SharedState   map[string]interface{} `json:"shared_state"`
}

type ServicePort struct {
	// Name is the name of the port from the workload specification
	Name string `json:"name"`
	// Port is the numeric port intended to be published
	Port int `json:"port"`
	// TargetPort is the port on the workload that hosts the actual traffic
	TargetPort int `json:"target_port"`
	// Protocol is TCP or UDP.
	Protocol score.ServicePortProtocol `json:"protocol"`
}

// NetworkService describes how to contact ports exposed by another workload
type NetworkService struct {
	ServiceName string                 `yaml:"service_name"`
	Ports       map[string]ServicePort `json:"ports"`
}

// ProvisionOutput is the output returned from a provisioner implementation.
type ProvisionOutput struct {
	ProvisionerUri  string                   `json:"-"`
	ResourceState   map[string]interface{}   `json:"resource_state"`
	ResourceOutputs map[string]interface{}   `json:"resource_outputs"`
	SharedState     map[string]interface{}   `json:"shared_state"`
	Manifests       []map[string]interface{} `json:"manifests"`

	// For testing and legacy reasons, built in provisioners can set a direct lookup function
	OutputLookupFunc framework.OutputLookupFunc `json:"-"`
}

type Provisioner interface {
	Uri() string
	Match(resUid framework.ResourceUid) bool
	Provision(ctx context.Context, input *Input) (*ProvisionOutput, error)
}

type ephemeralProvisioner struct {
	uri       string
	matchUid  framework.ResourceUid
	provision func(ctx context.Context, input *Input) (*ProvisionOutput, error)
}

func (e *ephemeralProvisioner) Uri() string {
	return e.uri
}

func (e *ephemeralProvisioner) Match(resUid framework.ResourceUid) bool {
	return resUid == e.matchUid
}

func (e *ephemeralProvisioner) Provision(ctx context.Context, input *Input) (*ProvisionOutput, error) {
	return e.provision(ctx, input)
}

// NewEphemeralProvisioner is mostly used for internal testing and uses the given provisioner function to provision an exact resource.
func NewEphemeralProvisioner(uri string, matchUid framework.ResourceUid, inner func(ctx context.Context, input *Input) (*ProvisionOutput, error)) Provisioner {
	return &ephemeralProvisioner{uri: uri, matchUid: matchUid, provision: inner}
}

// ApplyToStateAndProject takes the outputs of a provisioning request and applies to the state, file tree, and docker
// compose project.
func (po *ProvisionOutput) ApplyToStateAndProject(state *project.State, resUid framework.ResourceUid) (*project.State, error) {
	slog.Debug(
		fmt.Sprintf("Provisioned resource '%s'", resUid),
		"outputs", po.ResourceOutputs,
		"#manifests", len(po.Manifests),
	)

	out := *state
	out.Resources = maps.Clone(state.Resources)

	existing, ok := out.Resources[resUid]
	if !ok {
		return nil, fmt.Errorf("failed to apply to state - unknown res uid")
	}

	// Update the provisioner string
	existing.ProvisionerUri = po.ProvisionerUri

	// State must ALWAYS be updated. If we don't get state back, we assume it's now empty.
	if po.ResourceState != nil {
		existing.State = po.ResourceState
	} else {
		existing.State = make(map[string]interface{})
	}

	// Same with outputs, it must ALWAYS be updated.
	if po.ResourceOutputs != nil {
		existing.Outputs = po.ResourceOutputs
	} else {
		existing.Outputs = make(map[string]interface{})
	}

	if po.OutputLookupFunc != nil {
		existing.OutputLookupFunc = po.OutputLookupFunc
	}

	if po.SharedState != nil {
		out.SharedState = util.PatchMap(state.SharedState, po.SharedState)
	}

	// Manifests must also always be updated.
	if len(po.Manifests) > 0 {
		existing.Extras.Manifests = po.Manifests
	} else {
		existing.Extras.Manifests = make([]map[string]interface{}, 0)
	}

	out.Resources[resUid] = existing
	return &out, nil
}

func buildWorkloadServices(state *project.State) map[string]NetworkService {
	out := make(map[string]NetworkService, len(state.Workloads))
	for workloadName, workloadState := range state.Workloads {
		ns := NetworkService{
			ServiceName: convert.WorkloadServiceName(workloadName, state.Workloads[workloadName].Spec.Metadata),
			Ports:       make(map[string]ServicePort),
		}
		if workloadState.Spec.Service != nil {
			for s, port := range (*workloadState.Spec.Service).Ports {
				ns.Ports[s] = ServicePort{
					Name:       s,
					Port:       port.Port,
					TargetPort: util.DerefOr(port.TargetPort, port.Port),
					Protocol:   util.DerefOr(port.Protocol, score.ServicePortProtocolTCP),
				}
			}
			// Also add unique ports using a str-converted port number - this expands compatibility by allowing users
			// to indicate the named port using its port number as a secondary name.
			for s, port := range (*workloadState.Spec.Service).Ports {
				p2 := strconv.Itoa(port.Port)
				if _, ok := ns.Ports[p2]; !ok {
					ns.Ports[p2] = ns.Ports[s]
				}
			}
		}
		out[workloadName] = ns
	}
	return out
}

func ProvisionResources(ctx context.Context, state *project.State, provisioners []Provisioner) (*project.State, error) {
	out := state

	// provision in sorted order
	orderedResources, err := out.GetSortedResourceUids()
	if err != nil {
		return nil, fmt.Errorf("failed to determine sort order for provisioning: %w", err)
	}

	workloadServices := buildWorkloadServices(state)

	for _, resUid := range orderedResources {
		resState := out.Resources[resUid]
		provisionerIndex := slices.IndexFunc(provisioners, func(provisioner Provisioner) bool {
			return provisioner.Match(resUid)
		})
		if provisionerIndex < 0 {
			return nil, fmt.Errorf("resource '%s' is not supported by any provisioner", resUid)
		}
		provisioner := provisioners[provisionerIndex]
		if resState.ProvisionerUri != "" && resState.ProvisionerUri != provisioner.Uri() {
			return nil, fmt.Errorf("resource '%s' was previously provisioned by a different provider - undefined behavior", resUid)
		}

		var params map[string]interface{}
		if resState.Params != nil && len(resState.Params) > 0 {
			resOutputs, err := out.GetResourceOutputForWorkload(resState.SourceWorkload)
			if err != nil {
				return nil, fmt.Errorf("failed to find resource params for resource '%s': %w", resUid, err)
			}
			sf := framework.BuildSubstitutionFunction(out.Workloads[resState.SourceWorkload].Spec.Metadata, resOutputs)
			rawParams, err := framework.Substitute(resState.Params, sf)
			if err != nil {
				return nil, fmt.Errorf("failed to substitute params for resource '%s': %w", resUid, err)
			}
			params = rawParams.(map[string]interface{})
		}

		output, err := provisioner.Provision(ctx, &Input{
			ResourceGuid:     resState.Guid,
			ResourceUid:      string(resUid),
			ResourceType:     resUid.Type(),
			ResourceClass:    resUid.Class(),
			ResourceId:       resUid.Id(),
			ResourceParams:   params,
			ResourceMetadata: resState.Metadata,
			ResourceState:    resState.State,
			SourceWorkload:   resState.SourceWorkload,
			WorkloadServices: workloadServices,
			SharedState:      out.SharedState,
		})
		if err != nil {
			return nil, fmt.Errorf("resource '%s': failed to provision: %w", resUid, err)
		}

		output.ProvisionerUri = provisioner.Uri()
		out, err = output.ApplyToStateAndProject(out, resUid)
		if err != nil {
			return nil, fmt.Errorf("resource '%s': failed to apply outputs: %w", resUid, err)
		}
	}

	return out, nil
}
