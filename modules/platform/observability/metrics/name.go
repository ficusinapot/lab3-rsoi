package metrics

import "strings"

func metricNameComponent(value string) string {
	value = strings.Trim(strings.Map(func(r rune) rune {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if isLetter || isDigit || r == '_' {
			return r
		}
		return '_'
	}, value), "_")
	if value == "" {
		return "app"
	}
	if value[0] >= '0' && value[0] <= '9' {
		return "app_" + value
	}
	return value
}
