package charts

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// tableView returns the collapsed details element that holds a chart's data
// as a table. Values are written as the data holds them, not as the axis
// formats them. It returns "" when there are no rows or no fields.
func tableView(caption string, fields []string, rows []map[string]any) string {
	if len(rows) == 0 || len(fields) == 0 {
		return ""
	}
	summary := caption
	if summary == "" {
		summary = "Chart data"
	}
	var b strings.Builder
	b.WriteString(`<details class="container md2html-chart-data"><summary>`)
	b.WriteString(html.EscapeString(summary))
	b.WriteString(`</summary><div class="table-scroll"><table><thead><tr>`)
	for _, f := range fields {
		b.WriteString("<th>" + html.EscapeString(f) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, f := range fields {
			b.WriteString("<td>" + html.EscapeString(cellText(row[f])) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table></div></details>")
	return b.String()
}

// cellText renders one value as the data holds it. A nil value, including a
// missing field, is an empty cell.
func cellText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}
