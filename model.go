package main

import "strings"

// family maps a model id or alias to fable, opus, sonnet or haiku.
func family(model string) string {
	m := strings.ToLower(model)
	for _, f := range []string{"fable", "mythos", "opus", "sonnet", "haiku"} {
		if strings.Contains(m, f) {
			if f == "mythos" {
				return "fable"
			}
			return f
		}
	}
	return "opus"
}
