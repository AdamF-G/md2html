package charts

// mergeConfig lays theme under the spec's own config: a key the spec sets
// wins, and nested objects merge key by key. It returns the spec with the
// merged config; theme is not modified.
func mergeConfig(spec any, theme map[string]any) any {
	s, ok := spec.(map[string]any)
	if !ok {
		return spec
	}
	own, _ := s["config"].(map[string]any)
	s["config"] = deepMerge(theme, own)
	return s
}

// deepMerge returns base overlaid with over. Neither argument is modified.
func deepMerge(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		if m, ok := v.(map[string]any); ok {
			v = deepMerge(m, nil)
		}
		out[k] = v
	}
	for k, v := range over {
		bm, bok := out[k].(map[string]any)
		om, ook := v.(map[string]any)
		if bok && ook {
			out[k] = deepMerge(bm, om)
			continue
		}
		out[k] = v
	}
	return out
}
