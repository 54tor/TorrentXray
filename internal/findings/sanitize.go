package findings

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const maxTextRunes = 300

var controlCharacters = regexp.MustCompile(`[\x00-\x1F\x7F-\x9F\x{200B}\x{200E}\x{200F}\x{2028}-\x{202E}\x{2066}-\x{2069}\x{FEFF}]+`)

func mapOf(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func listOf(value any) []any {
	list, _ := value.([]any)
	return list
}

func limitedList(value any, maxCount int) []any {
	list := listOf(value)
	if len(list) > maxCount {
		return list[:maxCount]
	}
	return list
}

func numberOf(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	}
	return 0, false
}

func integerOf(value any) int {
	number, isNumeric := numberOf(value)
	if !isNumeric || math.Abs(number) >= math.MaxInt64 {
		return 0
	}
	return int(number)
}

func nonNegativeIntegerOf(value any) int {
	return max(0, integerOf(value))
}

func decimalOf(value any) float64 {
	number, _ := numberOf(value)
	return number
}

func nonNegativeDecimalOf(value any) float64 {
	return math.Max(0, decimalOf(value))
}

func textOf(value any) string {
	var text string
	switch typed := value.(type) {
	case string:
		text = typed
	case float64:
		text = strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			text = "1"
		}
	}
	text = strings.TrimSpace(controlCharacters.ReplaceAllString(text, " "))
	runes := []rune(text)
	if len(runes) > maxTextRunes {
		return string(runes[:maxTextRunes])
	}
	return text
}

func optionalTextOf(object map[string]any, key string) *string {
	value, present := object[key]
	if !present || value == nil {
		return nil
	}
	text := textOf(value)
	return &text
}

func textsOf(value any, maxCount int) []string {
	texts := []string{}
	for _, rawText := range limitedList(value, maxCount) {
		texts = append(texts, textOf(rawText))
	}
	return texts
}

func enumOf(value any, allowed []string, fallback string) string {
	text := textOf(value)
	if slices.Contains(allowed, text) {
		return text
	}
	return fallback
}

func dateOf(value any) time.Time {
	parsed, err := time.Parse(time.RFC3339, textOf(value))
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}

func nestingDepth(value any) int {
	deepestChild := 0
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			deepestChild = max(deepestChild, nestingDepth(child))
		}
	case []any:
		for _, child := range typed {
			deepestChild = max(deepestChild, nestingDepth(child))
		}
	default:
		return 0
	}
	return deepestChild + 1
}
