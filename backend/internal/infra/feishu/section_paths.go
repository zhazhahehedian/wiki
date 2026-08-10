package feishu

import "strings"

// citationSectionPaths keeps normal headings stable while making duplicate
// visible labels deterministic and unique for exact citation matching.
func citationSectionPaths(labels, identities []string) []string {
	normalized := make([]string, len(labels))
	counts := make(map[string]int, len(labels))
	for i, label := range labels {
		normalized[i] = strings.TrimSpace(label)
		counts[normalized[i]]++
	}
	paths := make([]string, len(labels))
	used := make(map[string]bool, len(labels))
	for i, label := range normalized {
		candidate := label
		if counts[label] > 1 || used[candidate] {
			candidate += " [" + identities[i] + "]"
		}
		for used[candidate] {
			candidate += " [" + identities[i] + "]"
		}
		paths[i] = candidate
		used[candidate] = true
	}
	return paths
}
