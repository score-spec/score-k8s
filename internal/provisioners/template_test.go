package provisioners_test

import (
	"testing"

	"github.com/score-spec/score-k8s/internal/provisioners"
	"github.com/stretchr/testify/assert"
)

func TestRenderTemplate_EmptyTemplate(t *testing.T) {
	data := provisioners.TemplateData{}
	out, err := provisioners.RenderTemplate("", data)
	if !assert.Equal(t, "", out) && err == nil {
		t.Errorf("expected error for empty template")
	}
}

func TestRenderTemplate_RenderingWorks(t *testing.T) {
	raw := "Hello {{ .Guid }}"
	data := provisioners.TemplateData{
		Guid: "some-guid",
	}
	out, err := provisioners.RenderTemplate(raw, data)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Hello some-guid" {
		t.Errorf("expected 'Hello some-guid', got %s", out)
	}
}

func TestRenderTemplate_RenderingWorksWithInvalidTemplateSyntax(t *testing.T) {
	raw := "Hello {{ invalid syntax }}"
	data := provisioners.TemplateData{}
	out, err := provisioners.RenderTemplate(raw, data)
	if err == nil {
		t.Errorf("expected error for invalid template syntax")
	}
	if !assert.Equal(t, "", out) && err != nil {
		t.Errorf("unexpected output: %s", out)
	}
}
