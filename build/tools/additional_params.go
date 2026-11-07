package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// validateAdditionalParamsOverlay rejects an override of a setting the production
// additional_params does not declare, so a typo fails the build and additional_params
// stays the complete inventory of settings with their production values.
func validateAdditionalParamsOverlay(base, overlay map[string]json.RawMessage) error {
	var unknown []string
	for name := range overlay {
		if _, ok := base[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return fmt.Errorf("additional_params_dev overrides settings missing from additional_params: %s (declare the setting with its production value first)", strings.Join(unknown, ", "))
}

// mergeAdditionalParams applies the dev overlay over the production additional_params.
func mergeAdditionalParams(base, overlay map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if len(overlay) == 0 {
		return base, nil
	}

	merged := make(map[string]json.RawMessage, len(base))
	for name, value := range base {
		merged[name] = value
	}
	for name, override := range overlay {
		value, err := mergeJSONValues(base[name], override)
		if err != nil {
			return nil, fmt.Errorf("additional_params_dev %q: %w", name, err)
		}
		merged[name] = value
	}
	return merged, nil
}

// mergeJSONValues merges two JSON objects field by field and replaces anything else
// outright. A field the base object does not have is added: the must-exist rule is a
// typo guard for setting names, not for the fields inside one setting's parameters.
func mergeJSONValues(base, overlay json.RawMessage) (json.RawMessage, error) {
	baseObject, baseWasString, baseOK := decodeJSONObject(base)
	overlayObject, _, overlayOK := decodeJSONObject(overlay)
	if !baseOK || !overlayOK {
		return overlay, nil
	}

	for name, override := range overlayObject {
		value, err := mergeJSONValues(baseObject[name], override)
		if err != nil {
			return nil, err
		}
		baseObject[name] = value
	}

	// Re-encoding normalizes whitespace and sorts keys inside the value; every consumer
	// unmarshals it, so only the rendered text differs from the production config.
	encoded, err := json.Marshal(baseObject)
	if err != nil {
		return nil, err
	}
	if !baseWasString {
		return encoded, nil
	}
	// The *_params settings carry their object inside a JSON string; keep that form so
	// the runtime schema of the setting is unchanged.
	quoted, err := json.Marshal(string(encoded))
	if err != nil {
		return nil, err
	}
	return quoted, nil
}

// decodeJSONObject decodes a JSON object, either given directly or encoded in a JSON
// string as the *_params settings are, reporting which of the two forms it was.
func decodeJSONObject(raw json.RawMessage) (object map[string]json.RawMessage, wasString bool, ok bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, false
	}

	switch trimmed[0] {
	case '{':
		if err := json.Unmarshal(trimmed, &object); err != nil {
			return nil, false, false
		}
		return object, false, true
	case '"':
		var unquoted string
		if err := json.Unmarshal(trimmed, &unquoted); err != nil {
			return nil, false, false
		}
		inner := bytes.TrimSpace([]byte(unquoted))
		if len(inner) == 0 || inner[0] != '{' {
			return nil, false, false
		}
		if err := json.Unmarshal(inner, &object); err != nil {
			return nil, false, false
		}
		return object, true, true
	}
	return nil, false, false
}

// addnodePeerPattern limits an env-supplied peer to one host[:port] token, so the value
// cannot inject further lines or options into the rendered backend config.
var addnodePeerPattern = regexp.MustCompile(`^[A-Za-z0-9.:\[\]-]+$`)

// appendBackendAddnodes appends private peers after the public addnode list of the coin
// config, keeping their order and dropping duplicates.
func appendBackendAddnodes(backend *Backend, raw string) error {
	peers := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	for i, peer := range peers {
		if !addnodePeerPattern.MatchString(peer) {
			// The peers are private, so name only the position of the bad entry.
			return fmt.Errorf("addnode peer #%d is not a host[:port]", i+1)
		}
	}
	if len(peers) == 0 {
		return nil
	}

	if backend.AdditionalParams == nil {
		backend.AdditionalParams = map[string]interface{}{}
	}
	params, ok := backend.AdditionalParams.(map[string]interface{})
	if !ok {
		return fmt.Errorf("backend additional_params is not an object")
	}

	var nodes []interface{}
	seen := make(map[string]struct{})
	switch existing := params["addnode"].(type) {
	case nil:
	case []interface{}:
		for _, node := range existing {
			name, ok := node.(string)
			if !ok {
				return fmt.Errorf("backend additional_params addnode must list strings")
			}
			nodes = append(nodes, name)
			seen[name] = struct{}{}
		}
	default:
		return fmt.Errorf("backend additional_params addnode must be a list")
	}
	for _, peer := range peers {
		if _, dup := seen[peer]; dup {
			continue
		}
		nodes = append(nodes, peer)
		seen[peer] = struct{}{}
	}
	params["addnode"] = nodes
	return nil
}
