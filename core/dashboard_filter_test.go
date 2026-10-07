package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

var dashToday = time.Date(2026, 9, 29, 0, 0, 0, 0, time.Local)

func dashPeriod() *DateRangeFilter {
	f := NewDateRangeFilter("period", 30)
	f.Today = func() time.Time { return dashToday }
	return f
}

func query(values map[string]string) func(string) string {
	return func(k string) string { return values[k] }
}

func TestDateRangeDefaultsToTheLastNDays(t *testing.T) {
	r := dashPeriod().Parse(query(nil)).(DateRange)
	if r.Start.Format("2006-01-02") != "2026-08-30" || r.End.Format("2006-01-02") != "2026-09-29" {
		t.Fatalf("got %v", r)
	}
}

func TestDateRangeFallsBackOnGarbageAndSwapsReversed(t *testing.T) {
	r := dashPeriod().Parse(query(map[string]string{"period_from": "nope", "period_to": "2026-09-29"})).(DateRange)
	if r.Start.Format("2006-01-02") != "2026-08-30" {
		t.Fatalf("got %v", r)
	}
	r = dashPeriod().Parse(query(map[string]string{"period_from": "2026-02-01", "period_to": "2026-01-01"})).(DateRange)
	if r.Start.Format("2006-01-02") != "2026-01-01" || r.End.Format("2006-01-02") != "2026-02-01" {
		t.Fatalf("got %v", r)
	}
}

func TestDateRangeQueryParamsRoundTrip(t *testing.T) {
	f := dashPeriod()
	v := f.Parse(query(map[string]string{"period_from": "2026-01-01", "period_to": "2026-01-31"}))
	p := f.QueryParams(v)
	if p["period_from"] != "2026-01-01" || p["period_to"] != "2026-01-31" {
		t.Fatalf("got %v", p)
	}
}

func TestSelectFilterParsesEmptyAsBlankAndResolvesChoices(t *testing.T) {
	f := NewSelectFilter("store", WithFilterChoicesFunc(func(ctx context.Context) ([]Choice, error) {
		return []Choice{{Value: "1", Label: "One"}}, nil
	}))
	if f.Parse(query(nil)).(string) != "" || len(f.QueryParams("")) != 0 {
		t.Fatal("blank select")
	}
	if f.Parse(query(map[string]string{"store": "7"})).(string) != "7" {
		t.Fatal("value")
	}
	choices, err := f.Choices(context.Background())
	if err != nil || len(choices) != 1 || choices[0].Label != "One" {
		t.Fatalf("got %v %v", choices, err)
	}
}

func TestWidgetKeyDefaultsToATransliteratedSlug(t *testing.T) {
	if WidgetKey(NewMetric("Статистика по проходам", func() any { return 1 })) != "statistika-po-prokhodam" {
		t.Fatal(WidgetKey(NewMetric("Статистика по проходам", func() any { return 1 })))
	}
	if WidgetKey(NewMetric("Users", func() any { return 1 }, WithKey("u"))) != "u" {
		t.Fatal("explicit key")
	}
}

func TestReloadsOnDefaultsToEveryFilter(t *testing.T) {
	names := []string{"period", "c"}
	if strings.Join(WidgetReloadsOn(NewMetric("A", nil), names), ",") != "period,c" {
		t.Fatal("default")
	}
	if len(WidgetReloadsOn(NewMetric("A", nil, WithDependsOn()), names)) != 0 {
		t.Fatal("none")
	}
	if strings.Join(WidgetReloadsOn(NewMetric("A", nil, WithDependsOn("c")), names), ",") != "c" {
		t.Fatal("some")
	}
}

func TestDescriptionFuncReceivesTheContext(t *testing.T) {
	w := NewMetric("A", nil, WithDescriptionFunc(func(dc DashboardContext) string { return "c=" + dc.String("c") }))
	if WidgetDescription(w, DashboardContext{Filters: map[string]any{"c": "7"}}) != "c=7" {
		t.Fatal("description")
	}
}

type ctxWidget struct{ baseWidget }

func (w ctxWidget) GetData() any { return nil }
func (w ctxWidget) Data(ctx context.Context, wc WidgetContext) (any, error) {
	return map[string]any{"Search": wc.Search}, nil
}

func TestContextWidgetsAreLazyAndResolveWithTheContext(t *testing.T) {
	w := ctxWidget{newBaseWidget("A", "x", nil)}
	if !IsLazyWidget(w) || IsLazyWidget(NewMetric("M", func() any { return 3 })) {
		t.Fatal("lazy detection")
	}
	data, err := ResolveWidgetData(context.Background(), w, WidgetContext{Search: "s"})
	if err != nil || data.(map[string]any)["Search"] != "s" {
		t.Fatalf("got %v %v", data, err)
	}
	data, _ = ResolveWidgetData(context.Background(), NewMetric("M", func() any { return 3 }), WidgetContext{})
	if data.(map[string]any)["Value"] != 3 {
		t.Fatal("legacy")
	}
}

func TestWidgetContextParsesOffsetAndSearch(t *testing.T) {
	d := Dashboard{Filters: []DashboardFilter{dashPeriod()}}
	wc := d.WidgetContext(NewMetric("A", nil), query(map[string]string{"offset": "50", "search": "acme"}), nil)
	if wc.Offset != 50 || wc.Search != "acme" || wc.Limit != 0 {
		t.Fatalf("got %+v", wc)
	}
	if d.WidgetContext(NewMetric("A", nil), query(map[string]string{"offset": "x"}), nil).Offset != 0 {
		t.Fatal("garbage")
	}
	if d.WidgetContext(NewMetric("A", nil), query(map[string]string{"offset": "-3"}), nil).Offset != 0 {
		t.Fatal("negative")
	}
}

func TestDashboardValidate(t *testing.T) {
	if err := (Dashboard{Widgets: []Widget{NewMetric("A", nil), NewMetric("A", nil)}}).Validate(); err == nil || !strings.Contains(err.Error(), "widget key") {
		t.Fatalf("got %v", err)
	}
	e := DashboardExport{Name: "x", Label: "X"}
	if err := (Dashboard{Exports: []DashboardExport{e, e}}).Validate(); err == nil || !strings.Contains(err.Error(), "export name") {
		t.Fatalf("got %v", err)
	}
	if err := (Dashboard{Widgets: []Widget{NewMetric("A", nil, WithDependsOn("nope"))}}).Validate(); err == nil || !strings.Contains(err.Error(), "unknown filter") {
		t.Fatalf("got %v", err)
	}
	mustPanic(t, "widget key", func() {
		New(WithDashboard(&Dashboard{Widgets: []Widget{NewMetric("A", nil), NewMetric("A", nil)}}))
	})
}

type denyExport struct{}

func (denyExport) Can(p *Principal, permission string, resource any) bool {
	return permission != "dashboard.export"
}

func TestVisibleExportsFollowPermission(t *testing.T) {
	d := Dashboard{Exports: []DashboardExport{{Name: "x", Label: "X"}}}
	if len(d.VisibleExports(nil, nil)) != 1 || len(d.VisibleExports(nil, denyExport{})) != 0 {
		t.Fatal("exports")
	}
}

func TestWidgetUnavailableIsAnError(t *testing.T) {
	var err error = &WidgetUnavailable{Message: "down"}
	var wu *WidgetUnavailable
	if !errors.As(err, &wu) || wu.Message != "down" {
		t.Fatal("errors.As")
	}
}
