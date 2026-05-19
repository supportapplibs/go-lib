package validation

import (
	"fmt"
	"strings"
)

// crossFieldValidators maps rule names to whether ALL colon-args are field references.
// false = only the first arg is a field, the rest are values (e.g. required_if:otherField,value).
var crossFieldValidators = map[string]bool{
	"required_with":        true,
	"required_with_all":    true,
	"required_without":     true,
	"required_without_all": true,
	"required_if":          false,
	"required_unless":      false,
}

// expandWildcardRules pre-processes rules that combine ".*" wildcard notation with
// cross-field validators (required_with, required_without, required_if, etc.).
//
// By default gookit resolves validator arguments against the top-level data map, so
// "komponen.*.minimumHarga": "required_with:maksimumHarga" would look for a top-level
// "maksimumHarga" key and never find the sibling inside each array element.
//
// This function expands such rules into per-index concrete rules before handing
// them to gookit, so sibling references are correctly qualified:
//
//	"komponen.*.minimumHarga": "numeric|required_with:maksimumHarga"
//
// becomes (for a two-element komponen array):
//
//	"komponen.0.minimumHarga": "numeric|required_with:komponen.0.maksimumHarga"
//	"komponen.1.minimumHarga": "numeric|required_with:komponen.1.maksimumHarga"
//
// Rules that do not contain a wildcard or do not reference cross-field validators
// are passed through unchanged so gookit handles them natively.
func expandWildcardRules(rules map[string]string, data map[string]any) map[string]string {
	result := make(map[string]string, len(rules))
	for field, rule := range rules {
		if !strings.Contains(field, ".*") || !hasCrossFieldValidator(rule) {
			result[field] = rule
			continue
		}

		prefix, suffix, _ := strings.Cut(field, ".*") // suffix e.g. ".minimumHarga" or ""

		arr := getSliceFromData(data, prefix)
		if arr == nil {
			// Cannot resolve the array — keep the original rule for gookit to handle.
			result[field] = rule
			continue
		}

		for i := range arr {
			concreteField := fmt.Sprintf("%s.%d%s", prefix, i, suffix)
			concreteRule := rewriteCrossFieldArgs(rule, prefix, i)
			result[concreteField] = concreteRule
		}
	}
	return result
}

// hasCrossFieldValidator reports whether any pipe-separated segment of rule is a
// cross-field validator.
func hasCrossFieldValidator(rule string) bool {
	for _, part := range strings.Split(rule, "|") {
		name, _, _ := strings.Cut(part, ":")
		if _, ok := crossFieldValidators[name]; ok {
			return true
		}
	}
	return false
}

// getSliceFromData traverses data using a dot-separated path and returns the
// resulting slice, or nil if the path does not resolve to a []any.
func getSliceFromData(data map[string]any, path string) []any {
	var current any = data
	for _, key := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[key]
	}
	arr, ok := current.([]any)
	if !ok {
		return nil
	}
	return arr
}

// rewriteCrossFieldArgs rewrites the field-name arguments inside cross-field
// validators so that bare sibling names become fully qualified paths.
func rewriteCrossFieldArgs(rule, prefix string, index int) string {
	parts := strings.Split(rule, "|")
	for i, part := range parts {
		name, args, hasColon := strings.Cut(part, ":")
		if !hasColon {
			continue
		}
		allFields, isCrossField := crossFieldValidators[name]
		if !isCrossField {
			continue
		}

		argParts := strings.Split(args, ",")
		if allFields {
			for j, arg := range argParts {
				argParts[j] = qualifySiblingField(arg, prefix, index)
			}
		} else if len(argParts) > 0 {
			// Only the first arg is a field reference; the rest are literal values.
			argParts[0] = qualifySiblingField(argParts[0], prefix, index)
		}
		parts[i] = name + ":" + strings.Join(argParts, ",")
	}
	return strings.Join(parts, "|")
}

// qualifySiblingField converts a bare field name (e.g. "maksimumHarga") into a
// fully qualified sibling path (e.g. "komponen.0.maksimumHarga").
// Fields that already contain "." or "*" are left unchanged.
func qualifySiblingField(field, prefix string, index int) string {
	if strings.ContainsAny(field, ".*") {
		return field
	}
	return fmt.Sprintf("%s.%d.%s", prefix, index, field)
}
