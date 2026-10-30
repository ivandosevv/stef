package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/pelletier/go-toml/v2"
)

// DecodeClientConfig reads the client's own JSON/JSONC/TOML schema for endpoint discovery.
func DecodeClientConfig(raw []byte, name string) (map[string]any, error) {
	client, err := LookupClient(name)
	if err != nil {
		return nil, err
	}
	if client.Name == "codex" {
		document := map[string]any{}
		if err := toml.Unmarshal(raw, &document); err != nil {
			return nil, err
		}
		return document, nil
	}
	return DecodeJSONConfig(raw, client.Name == "vscode")
}

// DecodeJSONConfig also accepts VS Code's JSON comments when allowComments is true.
// Numbers retain their original precision; no configuration is rewritten by this helper.
func DecodeJSONConfig(raw []byte, allowComments bool) (map[string]any, error) {
	document, _, err := decodeJSONClient(raw, allowComments)
	return document, err
}

func decodeJSONClient(raw []byte, allowComments bool) (map[string]any, []byte, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil, nil
	}
	normalized := raw
	var err error
	if allowComments {
		normalized, err = stripJSONComments(raw)
		if err != nil {
			return nil, nil, err
		}
	}
	data := normalized
	if allowComments {
		data = stripTrailingCommas(normalized)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, nil, fmt.Errorf("configuration must be a JSON object")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, nil, fmt.Errorf("configuration must contain exactly one JSON object")
	}
	return document, normalized, nil
}

// Replacing comments with spaces keeps byte offsets identical to the source.
func stripJSONComments(raw []byte) ([]byte, error) {
	data := bytes.Clone(raw)
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			end, err := quotedEnd(data, i)
			if err != nil {
				return nil, err
			}
			i = end
			continue
		}
		if data[i] != '/' || i+1 >= len(data) {
			continue
		}
		start := i
		switch data[i+1] {
		case '/':
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				data[i] = ' '
				i++
			}
			i--
		case '*':
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			if i+1 >= len(data) {
				return nil, fmt.Errorf("unterminated JSON comment")
			}
			i++
			for j := start; j <= i; j++ {
				if data[j] != '\n' && data[j] != '\r' {
					data[j] = ' '
				}
			}
		}
	}
	return data, nil
}

func stripTrailingCommas(raw []byte) []byte {
	data := bytes.Clone(raw)
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			end, err := quotedEnd(data, i)
			if err != nil {
				return data // The JSON decoder reports the malformed string.
			}
			i = end
		} else if data[i] == ',' {
			next := skipJSONSpace(data, i+1)
			if next < len(data) && (data[next] == '}' || data[next] == ']') {
				data[i] = ' '
			}
		}
	}
	return data
}

func quotedEnd(data []byte, start int) (int, error) {
	for i := start + 1; i < len(data); i++ {
		if data[i] == '\\' {
			i++
		} else if data[i] == '"' {
			return i, nil
		}
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

func skipJSONSpace(data []byte, start int) int {
	for start < len(data) && isJSONSpace(data[start]) {
		start++
	}
	return start
}

func isJSONSpace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\r'
}

func closingObject(data []byte, start int) (int, error) {
	depth := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '"':
			end, err := quotedEnd(data, i)
			if err != nil {
				return 0, err
			}
			i = end
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("missing JSON object end")
}

func serverObjectEnd(data []byte, key string) (int, bool, error) {
	depth, closing, found := 0, 0, false
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case '"':
			end, err := quotedEnd(data, i)
			if err != nil {
				return 0, false, err
			}
			next := skipJSONSpace(data, end+1)
			if depth == 1 && next < len(data) && data[next] == ':' {
				var name string
				_ = json.Unmarshal(data[i:end+1], &name)
				if name == key {
					if found {
						return 0, false, fmt.Errorf("duplicate %s configuration sections", key)
					}
					start := skipJSONSpace(data, next+1)
					if start >= len(data) || data[start] != '{' {
						return 0, false, fmt.Errorf("%s must be an object", key)
					}
					closing, err = closingObject(data, start)
					if err != nil {
						return 0, false, err
					}
					found = true
				}
			}
			i = end
		}
	}
	return closing, found, nil
}

// Insert only the new member into a JSONC file, preserving all existing source/comments.
func appendJSONCServer(raw, normalized []byte, key string, servers, desired map[string]any) ([]byte, error) {
	closing, found, err := serverObjectEnd(normalized, key)
	if err != nil {
		return nil, err
	}
	member, hasMembers := map[string]any{"stef": desired}, len(servers) > 0
	if !found {
		start := skipJSONSpace(normalized, 0)
		closing, err = closingObject(normalized, start)
		if err != nil {
			return nil, err
		}
		document, _, err := decodeJSONClient(raw, true)
		if err != nil {
			return nil, err
		}
		hasMembers = len(document) > 0
		member = map[string]any{key: map[string]any{"stef": desired}}
	}
	encoded, err := json.MarshalIndent(member, "", "  ")
	if err != nil {
		return nil, err
	}
	prefix := "\n"
	last := closing - 1
	for last >= 0 && isJSONSpace(normalized[last]) {
		last--
	}
	if hasMembers && (last < 0 || normalized[last] != ',') {
		prefix = ",\n"
	}
	addition := append([]byte(prefix), encoded[2:len(encoded)-2]...)
	addition = append(addition, '\n')
	next := append(bytes.Clone(raw[:closing]), addition...)
	next = append(next, raw[closing:]...)
	if _, _, err := decodeJSONClient(next, true); err != nil {
		return nil, fmt.Errorf("cannot safely extend JSONC configuration: %w", err)
	}
	return next, nil
}
