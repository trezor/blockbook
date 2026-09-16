//go:build unittest

package main

import (
	"strings"
	"testing"
)

type probeInner struct {
	Value string `json:"value"`
}

type probeOuter struct {
	Always *probeInner `json:"always" ts_nullable:"true"`
	Maybe  *probeInner `json:"maybe,omitempty"`
	Kind   string      `json:"kind" ts_type:"ProbeKind"`
}

type probeOmitEmpty struct {
	Bad *probeInner `json:"bad,omitempty" ts_nullable:"true"`
}

type probeNonPointer struct {
	Bad probeInner `json:"bad" ts_nullable:"true"`
}

func TestGenerateNullableAndAlias(t *testing.T) {
	out, err := generate([]interface{}{probeOuter{}}, []tsAlias{{"ProbeKind", "'a' | 'b'"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"export type ProbeKind = 'a' | 'b';\n",
		"\n    always: probeInner | null;\n",
		"\n    maybe?: probeInner;\n",
		"\n    kind: ProbeKind;\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.HasPrefix(out, header) {
		t.Errorf("output does not start with the header:\n%s", out)
	}
}

func TestGenerateRejectsUnreferencedAlias(t *testing.T) {
	_, err := generate([]interface{}{probeInner{}}, []tsAlias{{"Unused", "string"}})
	if err == nil || !strings.Contains(err.Error(), "Unused") {
		t.Fatalf("expected unreferenced alias error, got %v", err)
	}
}

func TestGenerateRejectsMisusedNullable(t *testing.T) {
	for _, root := range []interface{}{probeOmitEmpty{}, probeNonPointer{}} {
		_, err := generate([]interface{}{root}, nil)
		if err == nil || !strings.Contains(err.Error(), "ts_nullable") {
			t.Errorf("%T: expected ts_nullable misuse error, got %v", root, err)
		}
	}
}

func TestMarkNullableIsScopedToInterface(t *testing.T) {
	body := "\nexport interface A {\n    value?: string;\n}\nexport interface B {\n    value?: string;\n}"
	out, err := applyNullable(body, []nullableField{{iface: "B", json: "value"}}, "    ")
	if err != nil {
		t.Fatal(err)
	}
	want := "\nexport interface A {\n    value?: string;\n}\nexport interface B {\n    value: string | null;\n}"
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if _, err := applyNullable(body, []nullableField{{iface: "B", json: "missing"}}, "    "); err == nil {
		t.Error("expected error for a field the interface does not have")
	}
}
