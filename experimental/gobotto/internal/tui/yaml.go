package tui

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.yaml.in/yaml/v3"
)

type yamlSpan struct {
	start, end int
	color      string
}

// highlightYAML uses parsed scalar positions rather than treating colons or
// hashes inside strings as syntax. Input is sanitized before styles are added.
func highlightYAML(value string, width int) []string {
	raw := strings.Split(value, "\n")
	for i := range raw {
		raw[i] = displayText(ansi.Strip(raw[i]))
	}
	spans := make([][]yamlSpan, len(raw))
	var root yaml.Node
	if yaml.Unmarshal([]byte(strings.Join(raw, "\n")), &root) == nil {
		var visit func(*yaml.Node, bool)
		visit = func(node *yaml.Node, key bool) {
			if node.Kind == yaml.MappingNode {
				for i, child := range node.Content {
					visit(child, i%2 == 0)
				}
				return
			}
			if node.Kind != yaml.ScalarNode {
				for _, child := range node.Content {
					visit(child, false)
				}
				return
			}
			line, start := node.Line-1, node.Column-1
			if line < 0 || line >= len(raw) {
				return
			}
			runes := []rune(raw[line])
			if start < 0 || start >= len(runes) {
				return
			}
			end := len(runes)
			color := "#A7D8A5"
			if key {
				color = "#5EEAD4"
				// Metadata is emitted as block YAML. The mapping separator is
				// the final colon on a key-only line, or precedes the next value.
				quoted := rune(0)
				for i := start; i < len(runes); i++ {
					r := runes[i]
					if quoted == '"' && r == '\\' {
						i++
						continue
					}
					if quoted != 0 {
						if r == quoted {
							quoted = 0
						}
						continue
					}
					if r == '\'' || r == '"' {
						quoted = r
						continue
					}
					if r == ':' && (i+1 == len(runes) || runes[i+1] == ' ') {
						end = i
						break
					}
				}
			} else {
				switch node.Tag {
				case "!!int", "!!float":
					color = "#FCD34D"
				case "!!bool", "!!null":
					color = "#C4B5FD"
				}
			}
			spans[line] = append(spans[line], yamlSpan{start, end, color})
			if !key && node.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
				indent := len(raw[line]) - len(strings.TrimLeft(raw[line], " "))
				for i := line + 1; i < len(raw); i++ {
					if strings.TrimSpace(raw[i]) != "" && len(raw[i])-len(strings.TrimLeft(raw[i], " ")) <= indent {
						break
					}
					spans[i] = append(spans[i], yamlSpan{0, len([]rune(raw[i])), color})
				}
			}
		}
		visit(&root, false)
	}
	var lines []string
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#8593A4"))
	for i, line := range raw {
		runes := []rune(line)
		sort.SliceStable(spans[i], func(a, b int) bool { return spans[i][a].start < spans[i][b].start })
		var styled strings.Builder
		position := 0
		for _, span := range spans[i] {
			if span.start < position {
				continue
			}
			styled.WriteString(muted.Render(string(runes[position:span.start])))
			styled.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(span.color)).Render(string(runes[span.start:span.end])))
			position = span.end
		}
		styled.WriteString(muted.Render(string(runes[position:])))
		// Let the existing wrapper choose word boundaries. Cut each result from
		// the original styled line so continuation rows keep their token color.
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if len(spans[i]) > 0 {
			start := spans[i][len(spans[i])-1].start
			candidate := ansi.StringWidth(string(runes[:start]))
			if candidate < width-10 {
				indent = candidate
			}
		}
		indent = min(indent, max(0, width-1))
		prefix := ansi.Cut(styled.String(), 0, indent)
		body := ansi.Strip(ansi.Cut(styled.String(), indent, ansi.StringWidth(line)))
		consumed := 0
		for j, part := range strings.Split(ansi.Wrap(body, max(1, width-indent), ""), "\n") {
			start := strings.Index(body[consumed:], part) + consumed
			left := indent + ansi.StringWidth(body[:start])
			padding := strings.Repeat(" ", indent)
			if j == 0 {
				padding = prefix
			}
			lines = append(lines, padding+ansi.Cut(styled.String(), left, left+ansi.StringWidth(part)))
			consumed = start + len(part)
		}
	}
	return lines
}
