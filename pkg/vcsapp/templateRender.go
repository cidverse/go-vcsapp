package vcsapp

import (
	"bytes"
	"fmt"
	"text/template"
)

var templateFuncMap = template.FuncMap{
	"contains": func(container interface{}, elem interface{}) bool {
		switch c := container.(type) {
		case []string:
			for _, v := range c {
				if v == elem.(string) {
					return true
				}
			}
			return false
		default:
			return false
		}
	},
	"removeIssueMentions": RemoveIssueMentionFromMessage,
}

func Render(tpl string, data interface{}) ([]byte, error) {
	return RenderWithCustomDelimiter(tpl, "{{", "}}", data)
}

func RenderWithCustomDelimiter(tpl string, leftDelimiter string, rightDelimiter string, data interface{}) ([]byte, error) {
	return RenderCustom(tpl, leftDelimiter, rightDelimiter, data, nil)
}

func RenderCustom(tpl string, leftDelimiter string, rightDelimiter string, data interface{}, customFuncs template.FuncMap) ([]byte, error) {
	tmpl, err := template.New("template").Delims(leftDelimiter, rightDelimiter).Funcs(templateFuncMap).Funcs(customFuncs).Parse(tpl)
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
