package transformations

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// applyFilterPipeline processes an initial value through a chain of filters.
func applyFilterPipeline(initialVal any, exists bool, filters []string, ctx RenderContext) any {
	curr := initialVal
	hasVal := exists && curr != nil

	for _, rawFilter := range filters {
		fStr := strings.TrimSpace(rawFilter)
		if fStr == "" {
			continue
		}

		name, arg := parseFilterCall(fStr)
		nameLower := strings.ToLower(name)

		// Filters that handle nil/empty values explicitly
		switch nameLower {
		case "default":
			if !hasVal || isEmptyValue(curr) {
				curr = cleanFilterArg(arg)
				hasVal = true
			}
			continue
		case "coalesce":
			if !hasVal || isEmptyValue(curr) {
				candidates := splitFilterArgs(arg)
				for _, cand := range candidates {
					cTrim := strings.TrimSpace(cand)
					if strings.HasPrefix(cTrim, `"`) || strings.HasPrefix(cTrim, `'`) {
						cClean := cleanFilterArg(cTrim)
						if cClean != "" {
							curr = cClean
							hasVal = true
							break
						}
					} else {
						// Resolve path in context
						if val, ok := resolvePath(cTrim, ctx); ok && val != nil && !isEmptyValue(val) {
							curr = val
							hasVal = true
							break
						}
					}
				}
			}
			continue
		}

		if !hasVal {
			continue
		}

		curr = executeFilter(nameLower, arg, curr)
		hasVal = curr != nil
	}

	return curr
}

func executeFilter(name, arg string, val any) any {
	switch name {
	case "upper":
		return strings.ToUpper(fmt.Sprintf("%v", val))

	case "lower":
		return strings.ToLower(fmt.Sprintf("%v", val))

	case "trim":
		return strings.TrimSpace(fmt.Sprintf("%v", val))

	case "truncate":
		s := fmt.Sprintf("%v", val)
		args := splitFilterArgs(arg)
		limit := 0
		suffix := "..."
		if len(args) > 0 {
			limit, _ = strconv.Atoi(strings.TrimSpace(args[0]))
		}
		if len(args) > 1 {
			suffix = cleanFilterArg(args[1])
		}
		if limit > 0 && len(s) > limit {
			if limit > len(suffix) {
				return s[:limit-len(suffix)] + suffix
			}
			return s[:limit]
		}
		return s

	case "replace":
		args := splitFilterArgs(arg)
		s := fmt.Sprintf("%v", val)
		if len(args) >= 2 {
			oldStr := cleanFilterArg(args[0])
			newStr := cleanFilterArg(args[1])
			return strings.ReplaceAll(s, oldStr, newStr)
		} else if len(args) == 1 {
			oldStr := cleanFilterArg(args[0])
			return strings.ReplaceAll(s, oldStr, "")
		}
		return s

	case "split":
		sep := arg
		if sep == "" {
			sep = ","
		}
		s := fmt.Sprintf("%v", val)
		return strings.Split(s, sep)

	case "join":
		sep := arg
		switch slice := val.(type) {
		case []string:
			return strings.Join(slice, sep)
		case []any:
			strs := make([]string, len(slice))
			for i, v := range slice {
				strs[i] = fmt.Sprintf("%v", v)
			}
			return strings.Join(strs, sep)
		default:
			return fmt.Sprintf("%v", val)
		}

	case "json_encode":
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)

	case "url_encode":
		return url.QueryEscape(fmt.Sprintf("%v", val))

	case "base64":
		switch b := val.(type) {
		case []byte:
			return base64.StdEncoding.EncodeToString(b)
		case string:
			return base64.StdEncoding.EncodeToString([]byte(b))
		default:
			return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", val)))
		}

	case "date_format":
		layout := cleanFilterArg(arg)
		if layout == "" {
			layout = time.RFC3339
		}
		var t time.Time
		switch tv := val.(type) {
		case time.Time:
			t = tv
		case string:
			parsed, err := time.Parse(time.RFC3339, tv)
			if err != nil {
				parsed, err = time.Parse("2006-01-02 15:04:05", tv)
			}
			if err == nil {
				t = parsed
			}
		case int64:
			t = time.Unix(tv, 0)
		case int:
			t = time.Unix(int64(tv), 0)
		case float64:
			t = time.Unix(int64(tv), 0)
		}
		if t.IsZero() {
			return fmt.Sprintf("%v", val)
		}
		return t.Format(layout)

	case "number_format":
		decimals := 2
		if arg != "" {
			if d, err := strconv.Atoi(cleanFilterArg(arg)); err == nil && d >= 0 {
				decimals = d
			}
		}
		if num, ok := toFloat64(val); ok {
			format := fmt.Sprintf("%%.%df", decimals)
			return fmt.Sprintf(format, num)
		}
		return fmt.Sprintf("%v", val)

	case "regex_extract":
		pattern := cleanFilterArg(arg)
		if pattern == "" || len(pattern) > 256 {
			return ""
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return ""
		}
		s := fmt.Sprintf("%v", val)
		sub := re.FindStringSubmatch(s)
		if len(sub) > 1 {
			return sub[1]
		}
		return re.FindString(s)

	case "length":
		switch v := val.(type) {
		case string:
			return len(v)
		case []any:
			return len(v)
		case []string:
			return len(v)
		case map[string]any:
			return len(v)
		default:
			return 0
		}

	default:
		return val
	}
}

func parseFilterCall(filterStr string) (string, string) {
	if idx := strings.Index(filterStr, ":"); idx != -1 {
		name := strings.TrimSpace(filterStr[:idx])
		arg := strings.TrimSpace(filterStr[idx+1:])
		return name, cleanFilterArg(arg)
	}
	return filterStr, ""
}

func splitFilterArgs(arg string) []string {
	if arg == "" {
		return nil
	}
	parts := strings.Split(arg, ":")
	var result []string
	for _, p := range parts {
		result = append(result, strings.TrimSpace(p))
	}
	return result
}

func cleanFilterArg(s string) string {
	trimmed := strings.TrimSpace(s)
	if (strings.HasPrefix(trimmed, `"`) && strings.HasSuffix(trimmed, `"`)) ||
		(strings.HasPrefix(trimmed, `'`) && strings.HasSuffix(trimmed, `'`)) {
		if len(trimmed) >= 2 {
			return trimmed[1 : len(trimmed)-1]
		}
	}
	return trimmed
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val) == ""
	case bool:
		return !val
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}
