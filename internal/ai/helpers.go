package ai

// truncateError truncates an error response body for display.
// Never include full API responses in errors as they may contain sensitive data.
func truncateError(body []byte) string {
	s := string(body)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
