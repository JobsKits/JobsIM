package jobsim

import "strings"

func selfStatePublicPath(path string) bool {
	parts := strings.Split(strings.TrimPrefix(path, "/api/"), "/")
	if len(parts) != 3 || parts[0] != "conversations" {
		return false
	}
	switch parts[2] {
	case "read", "read-results", "view", "view-results", "draft", "draft-results":
		return true
	}
	return false
}
