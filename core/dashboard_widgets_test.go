package core

import (
	"context"
	"strings"
	"testing"
)

func TestMetricGroupReturnsTilesAndEmpty(t *testing.T) {
	g := NewMetricGroup("Overview", func(ctx context.Context, wc WidgetContext) ([]Tile, error) {
		return []Tile{{Label: "Doors", Value: 12, Icon: "door", Hint: wc.Search}}, nil
	})
	if !IsLazyWidget(g) {
		t.Fatal("lazy")
	}
	data, err := g.Data(context.Background(), WidgetContext{Search: "s"})
	m := data.(map[string]any)
	if err != nil || m["Empty"] != false || m["Tiles"].([]Tile)[0].Hint != "s" {
		t.Fatalf("got %v %v", data, err)
	}
	empty := NewMetricGroup("E", func(ctx context.Context, wc WidgetContext) ([]Tile, error) { return nil, nil })
	data, _ = empty.Data(context.Background(), WidgetContext{})
	if data.(map[string]any)["Empty"] != true {
		t.Fatal("empty")
	}
}

func TestNextOffset(t *testing.T) {
	items := func(n int) []map[string]any { return make([]map[string]any, n) }
	cases := []struct {
		rows          Rows
		offset, limit int
		want          int
		ok            bool
	}{
		{Rows{Items: items(50), Total: Total(120)}, 0, 50, 50, true},
		{Rows{Items: items(20), Total: Total(120)}, 100, 50, 0, false},
		{Rows{Items: items(50)}, 0, 50, 50, true},
		{Rows{Items: items(10)}, 0, 50, 0, false},
		{Rows{}, 0, 50, 0, false},
		{Rows{Items: items(50), Total: Total(50)}, 0, 50, 0, false},
		{Rows{Items: items(5), Total: Total(5)}, 0, 0, 0, false},
	}
	for i, c := range cases {
		got, ok := NextOffset(c.rows, c.offset, c.limit)
		if got != c.want || ok != c.ok {
			t.Fatalf("case %d: got %d %v", i, got, ok)
		}
	}
}

func TestDataTablePagesWithTheContextAndFormatsTheFooter(t *testing.T) {
	var seen WidgetContext
	table := NewDataTable("Contracts", []Column{{Key: "name", Label: "Name"}},
		func(ctx context.Context, wc WidgetContext) (Rows, error) {
			seen = wc
			return Rows{Items: []map[string]any{{"name": "a"}}, Total: Total(3), Totals: map[string]any{"name": "All"}}, nil
		})
	table.PageSizeValue = 1
	table.TotalLabel = "Total: {total}"
	data, err := table.Data(context.Background(), WidgetContext{Offset: 0, Limit: 1, Search: "x"})
	page := data.(DataTablePage)
	if err != nil || !page.HasNext || page.NextOffset != 1 || page.Footer != "Total: 3" || page.Empty || seen.Search != "x" {
		t.Fatalf("got %+v %v", page, err)
	}
	if table.PageSize() != 1 {
		t.Fatal("page size")
	}
}

func TestDataTableFirstPageWithoutRowsIsEmpty(t *testing.T) {
	table := NewDataTable("T", []Column{{Key: "a", Label: "A"}},
		func(ctx context.Context, wc WidgetContext) (Rows, error) { return Rows{Total: Total(0)}, nil })
	data, _ := table.Data(context.Background(), WidgetContext{Limit: 50})
	if !data.(DataTablePage).Empty {
		t.Fatal("empty")
	}
}

func TestDashboardValidateRejectsBadColumns(t *testing.T) {
	bad := NewDataTable("T", []Column{{Key: "a", Label: "A", Format: "money"}}, nil)
	if err := (Dashboard{Widgets: []Widget{bad}}).Validate(); err == nil || !strings.Contains(err.Error(), "money") {
		t.Fatalf("got %v", err)
	}
	bad = NewDataTable("T2", []Column{{Key: "a", Label: "A", Align: "center"}}, nil)
	if err := (Dashboard{Widgets: []Widget{bad}}).Validate(); err == nil || !strings.Contains(err.Error(), "center") {
		t.Fatalf("got %v", err)
	}
}

func TestADonutWithAContextSeriesIsLazyAndResolves(t *testing.T) {
	d := NewDonutCtx("Methods", func(ctx context.Context, wc WidgetContext) ([]ChartPoint, error) {
		return []ChartPoint{{Label: "Key", Value: 3}, {Label: "Face", Value: 1}}, nil
	})
	if !IsLazyWidget(d) || IsLazyWidget(NewDonut("Static", func() []ChartPoint { return nil })) {
		t.Fatal("lazy detection")
	}
	data, err := ResolveWidgetData(context.Background(), d, WidgetContext{})
	if err != nil || data.(map[string]any)["Total"] != 4.0 {
		t.Fatalf("got %v %v", data, err)
	}
}

func TestTabsIsLazyWhenAPanelIsAndResolvesEveryPanel(t *testing.T) {
	donut := NewDonutCtx("Methods", func(ctx context.Context, wc WidgetContext) ([]ChartPoint, error) {
		return []ChartPoint{{Label: "Key", Value: 3}}, nil
	})
	tabs := NewTabs("Breakdowns", []TabPanel{{Label: "Methods", Widget: donut}, {Label: "Total", Widget: NewMetric("Total", func() any { return 7 })}})
	if !IsLazyWidget(tabs) || IsLazyWidget(NewTabs("Legacy", []TabPanel{{Label: "T", Widget: NewMetric("T", func() any { return 1 })}})) {
		t.Fatal("lazy detection")
	}
	data, err := ResolveWidgetData(context.Background(), tabs, WidgetContext{})
	panels := data.(TabsData).Panels
	if err != nil || len(panels) != 2 || panels[0].Data.(map[string]any)["Total"] != 3.0 || panels[1].Data.(map[string]any)["Value"] != 7 {
		t.Fatalf("got %+v %v", data, err)
	}
}
