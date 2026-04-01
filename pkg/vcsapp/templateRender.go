package vcsapp

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/template"
)

var templateFuncMap = template.FuncMap{
	"contains": func(container interface{}, elem interface{}) bool {
		v := reflect.ValueOf(container)

		var query string
		e := reflect.ValueOf(elem)
		if e.Kind() == reflect.String {
			query = e.String()
		} else {
			query = fmt.Sprintf("%v", elem)
		}

		switch v.Kind() {
		case reflect.String:
			return strings.Contains(v.String(), query)
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				item := v.Index(i)

				var itemStr string
				if item.Kind() == reflect.String {
					itemStr = item.String()
				} else {
					itemStr = fmt.Sprintf("%v", item.Interface())
				}

				if itemStr == query {
					return true
				}
			}
			return false

		default:
			return false
		}
	},
	"join": func(arr []string, sep string) string {
		return strings.Join(arr, sep)
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
