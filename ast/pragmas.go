package ast

import (
	"fmt"
	"strings"
)

var PragmaKey = "pragma"

func parsePragmas(comments Comments) map[string]string {
	var pragmas map[string]string
	for i, l := 0, comments.Len(); i < l; i++ {
		c := comments.Index(i)
		text := strings.TrimSpace(c.RawText())
		prefix := fmt.Sprintf("//%s:", PragmaKey)
		if text, ok := strings.CutPrefix(text, prefix); ok {
			parts := strings.SplitN(text, " ", 2)
			var key, val string
			if len(parts) == 2 {
				key, val = parts[0], parts[1]
			} else {
				key = parts[0]
			}
			if pragmas == nil {
				pragmas = make(map[string]string)
			}
			pragmas[key] = val
		}
	}
	return pragmas
}
