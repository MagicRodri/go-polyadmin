package core

import "testing"

func TestMetricData(t *testing.T) {
	widget := NewMetric("Users", func() any { return 42 })
	data := widget.GetData().(map[string]any)
	if data["Value"] != 42 {
		t.Fatalf("got %v", data)
	}
}

func TestProgressPercent(t *testing.T) {
	widget := NewProgress("Tasks", func() (float64, float64) { return 25, 100 })
	data := widget.GetData().(map[string]any)
	if data["Percent"] != 25 {
		t.Fatalf("got %v", data)
	}
}

func TestProgressCapsAt100Percent(t *testing.T) {
	widget := NewProgress("Tasks", func() (float64, float64) { return 150, 100 })
	data := widget.GetData().(map[string]any)
	if data["Percent"] != 100 {
		t.Fatalf("got %v", data)
	}
}

func TestProgressZeroTargetIsZeroPercent(t *testing.T) {
	widget := NewProgress("Tasks", func() (float64, float64) { return 5, 0 })
	data := widget.GetData().(map[string]any)
	if data["Percent"] != 0 {
		t.Fatalf("got %v", data)
	}
}

func TestTableRows(t *testing.T) {
	widget := NewTable("Recent", []string{"ID", "Email"}, func() []map[string]any {
		return []map[string]any{{"ID": 1, "Email": "a@example.com"}}
	})
	data := widget.GetData().(map[string]any)
	rows := data["Rows"].([]map[string]any)
	if len(rows) != 1 || rows[0]["Email"] != "a@example.com" {
		t.Fatalf("got %v", rows)
	}
}

func TestChartComputesPercentagesRelativeToMax(t *testing.T) {
	widget := NewChart("Signups", func() []ChartPoint {
		return []ChartPoint{{Label: "Mon", Value: 10}, {Label: "Tue", Value: 20}, {Label: "Wed", Value: 5}}
	})
	data := widget.GetData().(map[string]any)
	series := data["Series"].([]chartRow)
	if series[0].Percent != 50 || series[1].Percent != 100 || series[2].Percent != 25 {
		t.Fatalf("got %v", series)
	}
}

func TestChartEmptySeries(t *testing.T) {
	widget := NewChart("Signups", func() []ChartPoint { return nil })
	data := widget.GetData().(map[string]any)
	series := data["Series"].([]chartRow)
	if len(series) != 0 {
		t.Fatalf("got %v", series)
	}
}

func TestActivityEntries(t *testing.T) {
	widget := NewActivity("Feed", func() []string { return []string{"User created", "User deleted"} })
	data := widget.GetData().(map[string]any)
	if entries := data["Entries"].([]string); len(entries) != 2 {
		t.Fatalf("got %v", entries)
	}
}

func TestStatReportsDirectionAndUnsignedDelta(t *testing.T) {
	for _, tc := range []struct {
		delta     float64
		direction string
		want      float64
	}{
		{12.5, "up", 12.5},
		{-4.26, "down", 4.3},
		{0, "flat", 0},
	} {
		widget := NewStat("Sales", func() (any, float64) { return "$45,385", tc.delta })
		data := widget.GetData().(map[string]any)
		if data["Direction"] != tc.direction || data["Delta"] != tc.want {
			t.Fatalf("delta %v: got %v", tc.delta, data)
		}
		if data["Value"] != "$45,385" {
			t.Fatalf("got %v", data)
		}
	}
}

func TestTimelineEntries(t *testing.T) {
	widget := NewTimeline("Latest activity", func() []TimelineEntry {
		return []TimelineEntry{{Time: "2h ago", Title: "User created", Description: "a@example.com"}}
	})
	data := widget.GetData().(map[string]any)
	entries := data["Entries"].([]TimelineEntry)
	if len(entries) != 1 || entries[0].Time != "2h ago" || entries[0].Description != "a@example.com" {
		t.Fatalf("got %v", entries)
	}
}

func TestTabsExposesPanelsForTheAdapterToRender(t *testing.T) {
	products := NewTable("Top products", []string{"Name"}, func() []map[string]any { return nil })
	widget := NewTabs("Statistics", []TabPanel{{Label: "Products", Widget: products}})

	// Panels() is what the adapter walks to render each child; GetData
	// reports the same panels under the key tabs.html reads.
	if panels := widget.Panels(); len(panels) != 1 || panels[0].Widget.Title() != "Top products" {
		t.Fatalf("got %v", panels)
	}
	data := widget.GetData().(map[string]any)
	if panels := data["Panels"].([]TabPanel); len(panels) != 1 || panels[0].Label != "Products" {
		t.Fatalf("got %v", panels)
	}
}

func TestTabsSatisfiesContainer(t *testing.T) {
	var _ Container = NewTabs("Statistics", nil)
}
