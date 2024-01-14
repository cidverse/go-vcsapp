package api

import (
	"strings"
)

// UnifyLineEndings replaces all line endings with LF
func UnifyLineEndings(str string) string {
	str = strings.ReplaceAll(str, "\r\n", "\n")
	return str
}
