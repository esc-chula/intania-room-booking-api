package auth

import "strings"

// ParseOSStudentIDs validates the entire allowlist before the bootstrap command writes anything.
func ParseOSStudentIDs(raw, suffix string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	seen := make(map[string]bool)
	var ids []string
	for part := range strings.SplitSeq(raw, ",") {
		id := strings.TrimSpace(part)
		if !numericStudentID(id) || !strings.HasSuffix(id, suffix) {
			return nil, ErrInvalidInput
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}
