package presentation

import (
	"html"
	"regexp"
	"strings"
)

var (
	inheritDoc = regexp.MustCompile(`(?i)\{@inheritdoc\}`)
	// {@link target text} and {@see target text}; the text is optional.
	inlineTag = regexp.MustCompile(`\{@(?:link|see)\s+([^\s}]+)(?:\s+([^}]*?))?\s*\}`)
	anchor    = regexp.MustCompile(`(?is)<a\s[^>]*?href\s*=\s*"([^"]*)"[^>]*>(.*?)</a>`)
	lineBreak = regexp.MustCompile(`(?i)<br\s*/?>[ \t]*\n?`)
	// Only HTML tag names are matched, so "array<string, mixed>" in a
	// sentence is not taken for a tag.
	htmlTag    = regexp.MustCompile(`(?i)<(/?)(b|strong|i|em|code|p|ul|ol|li|pre|span|div|tt|sup|sub|small|u|s|table|tr|td|th|tbody|thead|blockquote|dl|dt|dd|h[1-6]|hr|img|a)\b[^>]*>`)
	blankLines = regexp.MustCompile(`\n{3,}`)
)

// docMarkdown turns the HTML and inline tags PHPDoc comments use, as the
// PHP stubs do throughout, into Markdown the editor renders.
func docMarkdown(doc string) string {
	s := inheritDoc.ReplaceAllString(doc, "")
	s = inlineTag.ReplaceAllStringFunc(s, func(m string) string {
		parts := inlineTag.FindStringSubmatch(m)
		target, text := parts[1], parts[2]
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			if text == "" {
				return "<" + target + ">"
			}
			return "[" + text + "](" + target + ")"
		}
		return "`" + target + "`"
	})
	s = anchor.ReplaceAllString(s, "[$2]($1)")
	s = lineBreak.ReplaceAllString(s, "\n")
	s = htmlTag.ReplaceAllStringFunc(s, func(m string) string {
		parts := htmlTag.FindStringSubmatch(m)
		closing, name := parts[1] == "/", strings.ToLower(parts[2])
		switch name {
		case "b", "strong":
			return "**"
		case "i", "em":
			return "*"
		case "code":
			return "`"
		case "p":
			return "\n\n"
		case "ul", "ol":
			if closing {
				return "\n"
			}
		case "li":
			if !closing {
				return "\n- "
			}
		case "pre":
			return "\n```\n"
		}
		return ""
	})
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
