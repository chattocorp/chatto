package treememory

// changeSchema gives each operation its own argument shape. Moves must not
// regenerate document content, and content writes must not invent field names.
func changeSchema() map[string]any {
	fields := map[string]any{}
	for _, name := range []string{"id", "parent", "title", "summary", "type", "body"} {
		fields[name] = map[string]any{"type": "string"}
	}
	for _, name := range []string{"revision", "position", "version"} {
		fields[name] = map[string]any{"type": "integer"}
	}
	fields["data"] = map[string]any{"type": "object"}
	fields["instructions"] = map[string]any{"type": "boolean", "description": "Owner-approved operating guidance, not ordinary contributed data. Defaults to false on create."}
	var variants []any
	for _, op := range []string{"create", "update", "migrate", "move", "archive", "restore", "type"} {
		var names, required []string
		switch op {
		case "create":
			required = []string{"parent", "title", "summary", "type", "version", "data", "body"}
			names = append(append([]string{}, required...), "position", "instructions")
		case "update", "migrate":
			required = []string{"id", "revision", "title", "summary", "type", "version", "data", "body", "instructions"}
			names = required
		case "move":
			required = []string{"id", "revision", "parent", "position"}
			names = required
		case "archive", "restore":
			required = []string{"id", "revision"}
			names = required
		}
		props := map[string]any{"operation": map[string]any{"type": "string", "enum": []string{op}}}
		key := "node"
		if op == "type" {
			key = "type"
			props[key] = map[string]any{"type": "object", "properties": map[string]any{
				"name": map[string]any{"type": "string"}, "version": map[string]any{"type": "integer"}, "schema": map[string]any{"type": "object"},
			}, "required": []string{"name", "version", "schema"}, "additionalProperties": false}
		} else {
			nodeProps := map[string]any{}
			for _, name := range names {
				nodeProps[name] = fields[name]
			}
			props[key] = map[string]any{"type": "object", "properties": nodeProps, "required": required, "additionalProperties": false}
		}
		variants = append(variants, map[string]any{"type": "object", "properties": props, "required": []string{"operation", key}, "additionalProperties": false})
	}
	return map[string]any{"oneOf": variants}
}
