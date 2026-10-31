package trae

import (
	"sort"
	"time"
)

func now() int64 { return time.Now().Unix() }

func mergeToolCalls(calls map[int]map[string]string, items []any) {
	for _, item := range items {
		tc, ok := item.(map[string]any)
		if !ok {
			continue
		}
		idx := 0
		if v, ok := tc["index"].(float64); ok {
			idx = int(v)
		}
		fc, _ := tc["function_call"].(map[string]any)
		call, exists := calls[idx]
		if !exists {
			call = map[string]string{}
			calls[idx] = call
		}
		if id, _ := tc["id"].(string); id != "" {
			call["id"] = id
		}
		if fc != nil {
			if name, _ := fc["name"].(string); name != "" {
				call["name"] = name
			}
			if args, _ := fc["arguments"].(string); args != "" {
				call["arguments"] += args
			}
		}
	}
}

func sortedKeys(m map[int]map[string]string) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
