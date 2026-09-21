package core

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func noopHandler(ctx context.Context, ma ModelAdmin, objects []any, p *Principal) (string, error) {
	return "", nil
}

type placementAdmin struct{ BaseModelAdmin }

func newPlacementAdmin(actions []Action, detail []string) *placementAdmin {
	return &placementAdmin{BaseModelAdmin{
		ModelName: "User", SlugOverride: "users",
		DeclaredActions: actions, DeclaredDetailActions: detail,
	}}
}

func actionNames(actions []Action) []string {
	names := []string{}
	for _, a := range actions {
		names = append(names, a.Name)
	}
	return names
}

func TestActionWhereDefaultsToBoth(t *testing.T) {
	if got := NewAction("x", noopHandler).Where; got != ActionWhereBoth {
		t.Errorf("Where = %q, want %q", got, ActionWhereBoth)
	}
}

func TestActionWhereRejectsUnknownValues(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic")
		}
	}()
	NewAction("x", noopHandler, WithActionWhere("sidebar"))
}

func TestDeleteSelectedIsListOnly(t *testing.T) {
	if got := NewDeleteSelectedAction().Where; got != ActionWhereList {
		t.Errorf("Where = %q, want %q", got, ActionWhereList)
	}
}

func TestDefaultPlacementFollowsWhere(t *testing.T) {
	ma := newPlacementAdmin([]Action{
		NewAction("both", noopHandler),
		NewAction("only_list", noopHandler, WithActionWhere(ActionWhereList)),
		NewAction("only_detail", noopHandler, WithActionWhere(ActionWhereDetail)),
	}, nil)
	if got, want := actionNames(ActionsForList(ma)), []string{"both", "only_list", DeleteSelectedName}; !reflect.DeepEqual(got, want) {
		t.Errorf("list = %v, want %v", got, want)
	}
	if got, want := actionNames(ActionsForDetail(ma)), []string{"both", "only_detail"}; !reflect.DeepEqual(got, want) {
		t.Errorf("detail = %v, want %v", got, want)
	}
}

func TestDeleteSelectedNeverReachesTheDetailPage(t *testing.T) {
	for _, name := range actionNames(ActionsForDetail(newPlacementAdmin(nil, nil))) {
		if name == DeleteSelectedName {
			t.Error("delete_selected offered on the detail page")
		}
	}
}

func TestDetailActionsIsAnOrderedAllowlistThatOverridesWhere(t *testing.T) {
	ma := newPlacementAdmin([]Action{
		NewAction("a", noopHandler),
		NewAction("b", noopHandler, WithActionWhere(ActionWhereList)),
		NewAction("c", noopHandler),
	}, []string{"c", "b"})
	if got, want := actionNames(ActionsForDetail(ma)), []string{"c", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("detail = %v, want %v", got, want)
	}
	// The list page is unaffected by DetailActions.
	if got, want := actionNames(ActionsForList(ma)), []string{"a", "b", "c", DeleteSelectedName}; !reflect.DeepEqual(got, want) {
		t.Errorf("list = %v, want %v", got, want)
	}
}

func TestEmptyDetailActionsMeansNone(t *testing.T) {
	ma := newPlacementAdmin([]Action{NewAction("a", noopHandler)}, []string{})
	if got := ActionsForDetail(ma); len(got) != 0 {
		t.Errorf("detail = %v, want none", actionNames(got))
	}
}

func TestDeleteSelectedStaysOffTheDetailPageEvenWhenNamed(t *testing.T) {
	ma := newPlacementAdmin([]Action{NewAction("a", noopHandler)}, []string{"a", DeleteSelectedName})
	if got, want := actionNames(ActionsForDetail(ma)), []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("detail = %v, want %v", got, want)
	}
}

func TestAReplacementDeleteSelectedIsAlsoKeptOffTheDetailPage(t *testing.T) {
	ma := newPlacementAdmin([]Action{NewAction(DeleteSelectedName, noopHandler)}, nil)
	if got := actionNames(ActionsForDetail(ma)); len(got) != 0 {
		t.Errorf("detail = %v, want none", got)
	}
	if got := actionNames(ActionsForList(ma)); !reflect.DeepEqual(got, []string{DeleteSelectedName}) {
		t.Errorf("list = %v", got)
	}
}

func TestUnknownDetailActionPanicsAtRegistration(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic")
		}
		if !strings.Contains(fmt.Sprint(r), "nope") {
			t.Errorf("panic %v does not name the action", r)
		}
	}()
	New(WithModelAdmins(newPlacementAdmin([]Action{NewAction("a", noopHandler)}, []string{"a", "nope"})))
}
