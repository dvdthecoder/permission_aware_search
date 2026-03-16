package identifier

import (
	"encoding/json"
	"os"
	"strings"
)

func LoadThresholds(path string) QueryShapeThresholds {
	def := QueryShapeThresholds{
		ShortNoOpLen:        2,
		GenericPrefixMinLen: 3,
		IDPrefixMinLen:      2,
		EmailPrefixMinLen:   3,
	}
	if strings.TrimSpace(path) == "" {
		return def
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	out := def
	if err := json.Unmarshal(raw, &out); err != nil {
		return def
	}
	if out.ShortNoOpLen < 0 {
		out.ShortNoOpLen = def.ShortNoOpLen
	}
	if out.GenericPrefixMinLen <= 0 {
		out.GenericPrefixMinLen = def.GenericPrefixMinLen
	}
	if out.IDPrefixMinLen <= 0 {
		out.IDPrefixMinLen = def.IDPrefixMinLen
	}
	if out.EmailPrefixMinLen <= 0 {
		out.EmailPrefixMinLen = def.EmailPrefixMinLen
	}
	return out
}
