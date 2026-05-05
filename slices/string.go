package slices

// MergeUniqueStrings merges two string slices, preserving order and deduplicating entries.
// Elements from a appear first, followed by any elements from b not already in a.
func MergeUniqueStrings(a, b []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(a)+len(b))

	for _, v := range a {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}

	for _, v := range b {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}

	return result
}
