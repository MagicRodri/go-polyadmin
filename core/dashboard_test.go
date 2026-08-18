package core

import "testing"

type recordingAuthorizer struct {
	allowed map[string]bool
}

func (r recordingAuthorizer) Can(principal *Principal, permission string, resource any) bool {
	return r.allowed[permission]
}

func TestWidgetsWithoutPermissionAreAlwaysVisible(t *testing.T) {
	widget := NewMetric("Users", func() any { return 1 })
	dashboard := Dashboard{Widgets: []Widget{widget}}
	visible := dashboard.VisibleWidgets(nil, nil)
	if len(visible) != 1 {
		t.Fatalf("got %v", visible)
	}
}

func TestWidgetWithPermissionShownWhenNoAuthorizerConfigured(t *testing.T) {
	widget := NewMetric("Revenue", func() any { return 1 }, WithPermission("analytics.revenue.view"))
	dashboard := Dashboard{Widgets: []Widget{widget}}
	visible := dashboard.VisibleWidgets(nil, nil)
	if len(visible) != 1 {
		t.Fatalf("got %v", visible)
	}
}

func TestWidgetHiddenWhenAuthorizerDenies(t *testing.T) {
	widget := NewMetric("Revenue", func() any { return 1 }, WithPermission("analytics.revenue.view"))
	dashboard := Dashboard{Widgets: []Widget{widget}}
	visible := dashboard.VisibleWidgets(nil, recordingAuthorizer{allowed: map[string]bool{}})
	if len(visible) != 0 {
		t.Fatalf("got %v", visible)
	}
}

func TestMixedWidgetsOnlyOmitsTheDeniedOne(t *testing.T) {
	alwaysVisible := NewMetric("Users", func() any { return 1 })
	gated := NewMetric("Revenue", func() any { return 1 }, WithPermission("analytics.revenue.view"))
	dashboard := Dashboard{Widgets: []Widget{alwaysVisible, gated}}
	visible := dashboard.VisibleWidgets(nil, recordingAuthorizer{allowed: map[string]bool{}})
	if len(visible) != 1 || visible[0].Title() != alwaysVisible.Title() {
		t.Fatalf("got %v", visible)
	}
}
