package provisioners

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	util "github.com/score-spec/score-k8s/internal"
)

// Data is the structure sent to each template during rendering.
type TemplateData struct {
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
	WorkloadServices map[string]NetworkService
}

func RenderTemplate(tpl string, data TemplateData) (string, error) {
	if strings.TrimSpace(tpl) == "" {
		return "", nil
	}
	prepared, err := template.New("").
		Funcs(sprig.FuncMap()).
		Funcs(template.FuncMap{"encodeSecretRef": util.EncodeSecretReference}).
		Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	buff := new(bytes.Buffer)
	if err := prepared.Execute(buff, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	if strings.TrimSpace(buff.String()) == "" {
		return "", nil
	}
	return buff.String(), nil
}
