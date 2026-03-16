package semantic

import (
	"encoding/json"

	"permission_aware_search/internal/store"
)

func sourceForFilters(filters []store.Filter, source string) []FilterSource {
	out := make([]FilterSource, 0, len(filters))
	for _, f := range filters {
		out = append(out, FilterSource{Field: f.Field, Op: f.Op, Value: f.Value, Source: source})
	}
	return out
}

func mergeFilterSources(merged []store.Filter, groups ...[]FilterSource) []FilterSource {
	sourceMap := map[string]string{}
	for _, group := range groups {
		for _, fs := range group {
			key := filterSourceKey(fs.Field, fs.Op, fs.Value)
			if prior, ok := sourceMap[key]; ok {
				if prior != fs.Source {
					sourceMap[key] = "both"
				}
				continue
			}
			sourceMap[key] = fs.Source
		}
	}

	out := make([]FilterSource, 0, len(merged))
	for _, f := range merged {
		key := filterSourceKey(f.Field, f.Op, f.Value)
		src := sourceMap[key]
		if src == "" {
			src = "unknown"
		}
		out = append(out, FilterSource{Field: f.Field, Op: f.Op, Value: f.Value, Source: src})
	}
	return out
}

func filterSourceKey(field, op string, value interface{}) string {
	raw, _ := json.Marshal(value)
	return field + "|" + op + "|" + string(raw)
}
