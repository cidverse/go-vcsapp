package vcsapp

import (
	"bytes"
	"fmt"
	"text/template"
)

func Render(tpl string, data interface{}) ([]byte, error) {
	return RenderWithCustomDelimiter(tpl, "{{", "}}", data)
}

func RenderWithCustomDelimiter(tpl string, leftDelimiter string, rightDelimiter string, data interface{}) ([]byte, error) {
	tmpl, err := template.New("template").Delims(leftDelimiter, rightDelimiter).Parse(tpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var outputBuffer bytes.Buffer
	err = tmpl.Execute(&outputBuffer, data)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	return outputBuffer.Bytes(), nil
}
