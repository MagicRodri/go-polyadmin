package core

import (
	"reflect"
	"testing"
)

func TestNewAdminPageDefaults(t *testing.T) {
	page := NewAdminPage("/reports/contracts", noopPageHandler)
	if page.Label != "Contracts" {
		t.Fatalf("got label %q", page.Label)
	}
	if page.Permission != "page.reports.contracts" {
		t.Fatalf("got permission %q", page.Permission)
	}
	if !reflect.DeepEqual(page.HTTPMethods, []string{"GET", "POST"}) {
		t.Fatalf("got methods %v", page.HTTPMethods)
	}
	if page.HideFromNav {
		t.Fatalf("expected HideFromNav to default to false")
	}
	if page.Icon != "collection" {
		t.Fatalf("got icon %q", page.Icon)
	}
}

func TestNewAdminPageDefaultLabelHandlesHyphensAndUnderscores(t *testing.T) {
	page := NewAdminPage("/tools/import-users_now", noopPageHandler)
	if page.Label != "Import Users Now" {
		t.Fatalf("got label %q", page.Label)
	}
}

func TestPageOptionsOverrideDefaults(t *testing.T) {
	page := NewAdminPage(
		"/reports/contracts",
		noopPageHandler,
		WithPageLabel("Contracts Report"),
		WithPageCategory("Reports"),
		WithPageIcon("chart"),
		WithPagePermission("custom.permission"),
		WithPageMethods("GET"),
		WithPageHiddenFromNav(),
	)
	if page.Label != "Contracts Report" {
		t.Fatalf("got label %q", page.Label)
	}
	if page.Category != "Reports" {
		t.Fatalf("got category %q", page.Category)
	}
	if page.Icon != "chart" {
		t.Fatalf("got icon %q", page.Icon)
	}
	if page.Permission != "custom.permission" {
		t.Fatalf("got permission %q", page.Permission)
	}
	if !reflect.DeepEqual(page.HTTPMethods, []string{"GET"}) {
		t.Fatalf("got methods %v", page.HTTPMethods)
	}
	if !page.HideFromNav {
		t.Fatalf("expected HideFromNav to be true")
	}
}
