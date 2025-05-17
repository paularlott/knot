package util

// Checks if a string is in a slice of strings.
func InArray(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
