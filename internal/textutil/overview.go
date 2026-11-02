// Package textutil normalizes provider/NFO text for display and storage.
package textutil

import (
	"regexp"
	"strings"
)

var (
	reCRLF      = regexp.MustCompile(`\r\n?`)
	reHTMLBreak = regexp.MustCompile(`(?i)<br\s*/?>|</p\s*>|<p\s*>`)
	reSpaceRun  = regexp.MustCompile(`[ \t\x{3000}]{2,}`)
	reBlankRun  = regexp.MustCompile(`\n[ \t]*\n(?:[ \t]*\n)+`)
)

// NormalizeOverview turns the paragraph separators used by TMDb/NFO (blank lines,
// <br>/<p>, or runs of ideographic spaces such as "  \u3000\u3000") into real
// blank lines so the UI can render paragraphs. It trims outer whitespace.
func NormalizeOverview(s string) string {
	s = reCRLF.ReplaceAllString(s, "\n")
	s = reHTMLBreak.ReplaceAllString(s, "\n")
	s = reSpaceRun.ReplaceAllStringFunc(s, func(run string) string {
		// Two ordinary spaces are usually just a sentence gap; ideographic
		// spaces or a long run are paragraph separators.
		if strings.Contains(run, "\u3000") || len([]rune(run)) >= 4 {
			return "\n\n"
		}
		return " "
	})
	s = reBlankRun.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
