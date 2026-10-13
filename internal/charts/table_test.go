package charts

import (
	"strings"
	"testing"
)

func TestTableViewListsEncodedFieldsInOrder(t *testing.T) {
	got := tableView("Ratios", []string{"label", "v"}, []map[string]any{{"label": "a", "v": 1.5}, {"label": "b", "v": 2.0}})
	for _, want := range []string{"<summary>Ratios</summary>", "<th>label</th><th>v</th>", "<td>a</td><td>1.5</td>", "<td>b</td><td>2</td>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestTableViewNamesAnUncaptionedChart(t *testing.T) {
	if got := tableView("", []string{"v"}, []map[string]any{{"v": 1}}); !strings.Contains(got, "<summary>Chart data</summary>") {
		t.Errorf("got %s", got)
	}
}

func TestTableViewEscapesValues(t *testing.T) {
	got := tableView("", []string{"k"}, []map[string]any{{"k": `<i>&"`}})
	if strings.Contains(got, "<i>") || !strings.Contains(got, "&lt;i&gt;&amp;&#34;") {
		t.Errorf("value not escaped: %s", got)
	}
}
