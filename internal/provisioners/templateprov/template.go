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

package templateprov

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/score-spec/score-go/framework"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"

	util "github.com/score-spec/score-k8s/internal"
	"github.com/score-spec/score-k8s/internal/provisioners"
)

// Provisioner is the decoded template provisioner.
// A template provisioner provisions a resource by evaluating a series of Go text/templates that have access to some
// input parameters, previous state, and utility functions. Each parameter is expected to return a JSON object.
type Provisioner struct {
	ProvisionerUri string  `yaml:"uri"`
	ResType        string  `yaml:"type"`
	ResClass       *string `yaml:"class,omitempty"`
	ResId          *string `yaml:"id,omitempty"`
	ResDescription string  `yaml:"description,omitempty"`

	// The InitTemplate is always evaluated first, it is used as temporary or working set data that may be needed in the
	// later templates. It has access to the resource inputs and previous state.
	InitTemplate string `yaml:"init,omitempty"`
	// StateTemplate generates the new state of the resource based on the init and previous state.
	StateTemplate string `yaml:"state,omitempty"`
	// SharedStateTemplate generates modifications to the shared state, based on the init and current state.
	SharedStateTemplate string `yaml:"shared,omitempty"`
	// OutputsTemplate generates the outputs of the resource, based on the init and current state.
	OutputsTemplate string `yaml:"outputs,omitempty"`

	ManifestsTemplate string `yaml:"manifests,omitempty"`

	// SupportedParams is a list of parameters that the provisioner expects to be passed in.
	SupportedParams []string `yaml:"supported_params,omitempty"`

	// ExpectedOutputs is a list of expected outputs that the provisioner should return.
	ExpectedOutputs []string `yaml:"expected_outputs,omitempty"`
}

func Parse(raw map[string]interface{}) (*Provisioner, error) {
	p := new(Provisioner)
	intermediate, _ := yaml.Marshal(raw)
	dec := yaml.NewDecoder(bytes.NewReader(intermediate))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, err
	}
	if p.ProvisionerUri == "" {
		return nil, fmt.Errorf("uri not set")
	} else if p.ResType == "" {
		return nil, fmt.Errorf("type not set")
	}
	return p, nil
}

func (p *Provisioner) Uri() string {
	return p.ProvisionerUri
}

func (p *Provisioner) Match(resUid framework.ResourceUid) bool {
	if resUid.Type() != p.ResType {
		return false
	} else if p.ResClass != nil && resUid.Class() != *p.ResClass {
		return false
	} else if p.ResId != nil && resUid.Id() != *p.ResId {
		return false
	}
	return true
}

func (p *Provisioner) Description() string {
	return p.ResDescription
}

func (p *Provisioner) Class() string {
	if p.ResClass == nil {
		return "(any)"
	}
	return *p.ResClass
}

func (p *Provisioner) Type() string {
	return p.ResType
}

func (p *Provisioner) Params() []string {
	if p.SupportedParams == nil {
		return []string{}
	}
	params := make([]string, len(p.SupportedParams))
	copy(params, p.SupportedParams)
	slices.Sort(params)
	return params
}

func (p *Provisioner) Outputs() []string {
	if p.ExpectedOutputs == nil {
		return []string{}
	}
	outputs := make([]string, len(p.ExpectedOutputs))
	copy(outputs, p.ExpectedOutputs)
	slices.Sort(outputs)
	return outputs
}

func renderTemplateAndDecode(raw string, data interface{}, out interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	prepared, err := template.New("").
		Funcs(sprig.FuncMap()).
		Funcs(template.FuncMap{"encodeSecretRef": util.EncodeSecretReference}).
		Parse(raw)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}
	buff := new(bytes.Buffer)
	if err := prepared.Execute(buff, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	buffContents := buff.String()
	if strings.TrimSpace(buff.String()) == "" {
		return nil
	}
	var intermediate interface{}
	if err := yaml.Unmarshal([]byte(buffContents), &intermediate); err != nil {
		slog.Debug(fmt.Sprintf("template output was '%s' from template '%s'", buffContents, raw))
		return fmt.Errorf("failed to decode output: %w", err)
	}
	err = mapstructure.Decode(intermediate, &out)
	if err != nil {
		return fmt.Errorf("failed to decode output: %w", err)
	}
	return nil
}

// Data is the structure sent to each template during rendering.
type Data struct {
	// Guid is a random uuid generated the first time this resource is added to the project.
	Guid string

	// Uid is the combined id like type.class#id
	Uid string
	// Type is always defined as the resource type
	Type string
	// Class is the resource class, it defaults to 'default'
	Class string
	// Id is the resource id, like 'global-id' or 'workload.res-name'
	Id string

	Params   map[string]interface{}
	Metadata map[string]interface{}

	Init   map[string]interface{}
	State  map[string]interface{}
	Shared map[string]interface{}

	SourceWorkload   string
	WorkloadServices map[string]provisioners.NetworkService
	Namespace        string
}

func (p *Provisioner) Provision(ctx context.Context, input *provisioners.Input) (*provisioners.ProvisionOutput, error) {
	out := &provisioners.ProvisionOutput{}

	// The data payload that gets passed into each template
	data := Data{
		Guid:             input.ResourceGuid,
		Uid:              input.ResourceUid,
		Type:             input.ResourceType,
		Class:            input.ResourceClass,
		Id:               input.ResourceId,
		Params:           input.ResourceParams,
		Metadata:         input.ResourceMetadata,
		State:            input.ResourceState,
		Shared:           input.SharedState,
		SourceWorkload:   input.SourceWorkload,
		WorkloadServices: input.WorkloadServices,
		Namespace:        input.Namespace,
	}

	init := make(map[string]interface{})
	if err := renderTemplateAndDecode(p.InitTemplate, &data, &init); err != nil {
		return nil, fmt.Errorf("init template failed: %w", err)
	}
	data.Init = init

	out.ResourceState = make(map[string]interface{})
	if err := renderTemplateAndDecode(p.StateTemplate, &data, &out.ResourceState); err != nil {
		return nil, fmt.Errorf("state template failed: %w", err)
	}
	data.State = out.ResourceState

	out.SharedState = make(map[string]interface{})
	if err := renderTemplateAndDecode(p.SharedStateTemplate, &data, &out.SharedState); err != nil {
		return nil, fmt.Errorf("shared template failed: %w", err)
	}
	data.Shared = util.PatchMap(data.Shared, out.SharedState)

	out.ResourceOutputs = make(map[string]interface{})
	if err := renderTemplateAndDecode(p.OutputsTemplate, &data, &out.ResourceOutputs); err != nil {
		return nil, fmt.Errorf("outputs template failed: %w", err)
	}

	out.Manifests = make([]map[string]interface{}, 0)
	if err := renderTemplateAndDecode(p.ManifestsTemplate, &data, &out.Manifests); err != nil {
		return nil, fmt.Errorf("manifests template failed: %w", err)
	}

	// validate the manifests
	for i, manifest := range out.Manifests {
		raw, _ := yaml.Marshal(manifest)
		if _, _, err := util.YamlSerializerInfo.StrictSerializer.Decode(raw, nil, nil); err != nil && !runtime.IsNotRegisteredError(err) {
			return nil, fmt.Errorf("manifests.%d: matched a known kind but was not valid: %w", i, err)
		}
	}

	return out, nil
}

var _ provisioners.Provisioner = (*Provisioner)(nil)
