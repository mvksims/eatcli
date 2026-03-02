package wolt

import (
	"fmt"
	"net/url"
	"strings"
)

func firstStringFromScopes(scopes []map[string]interface{}, keys []string) string {
	for _, scope := range scopes {
		if value := firstStringFromScope(scope, keys); value != "" {
			return value
		}
	}
	return ""
}

func firstDisplayStringFromScopes(scopes []map[string]interface{}, keys []string) string {
	for _, scope := range scopes {
		if value := firstDisplayStringFromScope(scope, keys); value != "" {
			return value
		}
	}
	return ""
}

func firstStringFromScope(scope map[string]interface{}, keys []string) string {
	for _, key := range keys {
		raw, exists := scope[key]
		if !exists {
			continue
		}
		if text := toString(raw); text != "" {
			return text
		}
	}
	return ""
}

func firstDisplayStringFromScope(scope map[string]interface{}, keys []string) string {
	for _, key := range keys {
		raw, exists := scope[key]
		if !exists {
			continue
		}
		if text := toDisplayString(raw); text != "" {
			return text
		}
	}
	return ""
}

func firstAnyFromScope(scope map[string]interface{}, keys []string) (interface{}, bool) {
	for _, key := range keys {
		raw, exists := scope[key]
		if exists {
			return raw, true
		}
	}
	return nil, false
}

func extractPriceFromValue(value interface{}) (interface{}, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"amount", "value", "current", "price", "formatted", "display"} {
			if raw, ok := v[key]; ok {
				return raw, true
			}
		}
	case []interface{}:
		for _, item := range v {
			if nested, ok := extractPriceFromValue(item); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

func normalizePrice(value interface{}) interface{} {
	if nested, ok := extractPriceFromValue(value); ok {
		return nested
	}
	return value
}

func asMap(value interface{}) map[string]interface{} {
	if mapped, ok := value.(map[string]interface{}); ok {
		return mapped
	}
	return nil
}

func findNestedMapByKey(value interface{}, key string) map[string]interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		if candidate, ok := v[key]; ok {
			if mapped, ok := candidate.(map[string]interface{}); ok {
				return mapped
			}
		}
		for _, nested := range v {
			if result := findNestedMapByKey(nested, key); result != nil {
				return result
			}
		}
	case []interface{}:
		for _, nested := range v {
			if result := findNestedMapByKey(nested, key); result != nil {
				return result
			}
		}
	}
	return nil
}

func findNestedArrayByKey(value interface{}, key string) []interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		if candidate, ok := v[key]; ok {
			if arr, ok := candidate.([]interface{}); ok {
				return arr
			}
		}
		for _, nested := range v {
			if result := findNestedArrayByKey(nested, key); result != nil {
				return result
			}
		}
	case []interface{}:
		for _, nested := range v {
			if result := findNestedArrayByKey(nested, key); result != nil {
				return result
			}
		}
	}
	return nil
}

func collectStringValues(value interface{}) []string {
	values := make([]string, 0, 16)
	var walk func(interface{})

	walk = func(current interface{}) {
		switch v := current.(type) {
		case string:
			text := strings.TrimSpace(v)
			if text != "" {
				values = append(values, text)
			}
		case map[string]interface{}:
			for _, nested := range v {
				walk(nested)
			}
		case []interface{}:
			for _, nested := range v {
				walk(nested)
			}
		}
	}

	walk(value)
	return values
}

func fallbackNameFromTextValues(values []string) string {
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if strings.Contains(v, "://") || strings.HasPrefix(v, "/") {
			continue
		}
		if strings.Contains(strings.ToLower(v), "itemid-") {
			continue
		}
		if strings.HasPrefix(v, "€") || strings.HasPrefix(v, "$") || strings.HasPrefix(v, "£") {
			continue
		}
		if len(v) < 3 {
			continue
		}
		return v
	}
	return ""
}

func extractIDFromText(text string) string {
	candidates := []string{text}
	if decoded := decodeText(text); decoded != text {
		candidates = append(candidates, decoded)
	}

	for _, candidate := range candidates {
		if id := extractTokenAfterMarker(candidate, "itemid-"); id != "" {
			return id
		}
		if id := extractSegmentAfterMarker(candidate, "/item/"); id != "" {
			return id
		}
	}
	return ""
}

func extractVenueSlugFromText(text string) string {
	candidates := []string{text}
	if decoded := decodeText(text); decoded != text {
		candidates = append(candidates, decoded)
	}

	for _, candidate := range candidates {
		if slug := extractSegmentAfterMarker(candidate, "/venue/"); slug != "" {
			return slug
		}
	}
	return ""
}

func decodeText(text string) string {
	decoded, err := url.QueryUnescape(text)
	if err == nil {
		return decoded
	}
	return text
}

func extractTokenAfterMarker(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}

	start := idx + len(marker)
	end := start

	for end < len(text) {
		c := text[end]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' {
			end++
			continue
		}
		break
	}

	if end <= start {
		return ""
	}

	return text[start:end]
}

func extractSegmentAfterMarker(text, marker string) string {
	idx := strings.Index(strings.ToLower(text), strings.ToLower(marker))
	if idx < 0 {
		return ""
	}

	start := idx + len(marker)
	if start >= len(text) {
		return ""
	}

	end := start
	for end < len(text) {
		c := text[end]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' {
			end++
			continue
		}
		break
	}

	if end <= start {
		return ""
	}

	return text[start:end]
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	default:
		return ""
	}
}

func toDisplayString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		for _, key := range []string{"text", "value", "name", "title", "label"} {
			if raw, ok := v[key]; ok {
				if text := toDisplayString(raw); text != "" {
					return text
				}
			}
		}
		return ""
	case []interface{}:
		for _, item := range v {
			if text := toDisplayString(item); text != "" {
				return text
			}
		}
		return ""
	default:
		return toString(value)
	}
}
