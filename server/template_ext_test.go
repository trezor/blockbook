//go:build unittest

package server

import (
	"html/template"
	"testing"
)

func TestApplyTemplateFuncs_RegistersExtensions(t *testing.T) {
	m := template.FuncMap{}
	applyTemplateFuncs(m)
	if _, ok := m["chainExtra"]; !ok {
		t.Fatal("expected chainExtra to be registered in template func map")
	}
}

func TestApplyTemplateFuncs_CollisionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on function name collision")
		}
	}()
	m := template.FuncMap{
		"chainExtra": func() {},
	}
	applyTemplateFuncs(m)
}
