package control

import "slices"

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func subset(values, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}

	for _, value := range values {
		if !containsString(allowed, value) {
			return false
		}
	}

	return true
}
