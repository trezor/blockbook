package server

import (
	"fmt"
	"html/template"
)

var registeredTemplateFuncs = template.FuncMap{}

func registerTemplateFunc(name string, fn interface{}) {
	if name == "" {
		panic("template function name is empty")
	}
	if fn == nil {
		panic(fmt.Sprintf("template function %q is nil", name))
	}
	if _, exists := registeredTemplateFuncs[name]; exists {
		panic(fmt.Sprintf("template function %q is already registered", name))
	}
	registeredTemplateFuncs[name] = fn
}

func applyTemplateFuncs(dst template.FuncMap) {
	for name, fn := range registeredTemplateFuncs {
		if _, exists := dst[name]; exists {
			panic(fmt.Sprintf("template function %q collides with built-in function map", name))
		}
		dst[name] = fn
	}
}
