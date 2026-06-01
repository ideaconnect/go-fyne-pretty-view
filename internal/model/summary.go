package model

import "strconv"

// itemsSummary renders the collapsed-state count for JSON objects/arrays.
func itemsSummary(n int) string {
	if n == 1 {
		return "1 item"
	}
	return strconv.Itoa(n) + " items"
}

// childrenSummary renders the collapsed-state count for XML/HTML elements.
func childrenSummary(n int) string {
	if n == 1 {
		return "1 child"
	}
	return strconv.Itoa(n) + " children"
}

// summaryFor returns the muted collapsed-state text for a foldable node.
func summaryFor(k Kind, childCount int) string {
	switch k {
	case KindElement:
		return childrenSummary(childCount)
	default:
		return itemsSummary(childCount)
	}
}
