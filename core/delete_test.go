package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type previewItem struct {
	ID   int
	Name string
}

func (i previewItem) String() string { return "item " + i.Name }

// previewTarget is a registered resource a preview can point at.
type previewTarget struct{ BaseModelAdmin }

func newPreviewTarget(model, slug string) *previewTarget {
	return &previewTarget{BaseModelAdmin{ModelName: model, SlugOverride: slug, DisplayFields: []string{"ID", "Name"}}}
}

// previewSource is the ModelAdmin being deleted from; its preview is canned.
type previewSource struct {
	BaseModelAdmin
	preview DeletePreview
	err     error
	calls   int
	got     []any
}

func (s *previewSource) DeletePreview(ctx context.Context, objects []any) (DeletePreview, error) {
	s.calls++
	s.got = objects
	return s.preview, s.err
}

type previewAuthorizer func(permission string, resource any) bool

func (f previewAuthorizer) Can(principal *Principal, permission string, resource any) bool {
	return f(permission, resource)
}

func items(names ...string) []any {
	out := make([]any, len(names))
	for i, n := range names {
		out[i] = previewItem{ID: i + 1, Name: n}
	}
	return out
}

func newPreviewAdmin(source *previewSource, opts ...Option) *Admin {
	source.BaseModelAdmin = BaseModelAdmin{ModelName: "Organization", SlugOverride: "organizations"}
	all := append([]Option{WithModelAdmins(source, newPreviewTarget("User", "users"), newPreviewTarget("Invoice", "invoices"))}, opts...)
	return New(all...)
}

func TestNoCapabilityResolvesToNothing(t *testing.T) {
	plain := newPreviewTarget("User", "users")
	admin := New(WithModelAdmins(plain))
	got, err := ResolveDeletePreview(context.Background(), admin, plain, nil, items("a"))
	if err != nil || got.Blocked || len(got.Cascades) != 0 || len(got.Protected) != 0 {
		t.Fatalf("got %+v, %v", got, err)
	}
	if PreviewsDeletes(plain) {
		t.Error("PreviewsDeletes(plain) = true")
	}
}

func TestResolveCallsTheHostOnceWithTheObjects(t *testing.T) {
	source := &previewSource{}
	admin := newPreviewAdmin(source)
	objs := items("acme")
	if _, err := ResolveDeletePreview(context.Background(), admin, source, nil, objs); err != nil {
		t.Fatal(err)
	}
	if source.calls != 1 || !reflect.DeepEqual(source.got, objs) {
		t.Errorf("calls=%d got=%v", source.calls, source.got)
	}
	if !PreviewsDeletes(source) {
		t.Error("PreviewsDeletes(source) = false")
	}
}

func TestCascadeGroupIsSampledAndCounted(t *testing.T) {
	source := &previewSource{}
	admin := newPreviewAdmin(source)
	many := items("a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l")
	source.preview = DeletePreview{Cascades: []DeleteGroup{{Resource: "users", Objects: many, Total: 40}}}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	g := got.Cascades[0]
	if g.Label != "User" || g.Total != 40 || len(g.Visible) != DeletePreviewSample || g.More != 30 || g.Hidden != 0 {
		t.Errorf("got %+v", g)
	}
	if g.ModelAdmin == nil || g.ModelAdmin.Slug() != "users" {
		t.Errorf("ModelAdmin = %v", g.ModelAdmin)
	}
	if got.Blocked {
		t.Error("a permitted cascade blocked the delete")
	}
}

func TestTotalIsRaisedToTheSampleAndEmptyGroupsAreDropped(t *testing.T) {
	source := &previewSource{}
	admin := newPreviewAdmin(source)
	source.preview = DeletePreview{
		Cascades:  []DeleteGroup{{Resource: "users", Objects: items("a", "b")}, {Resource: "invoices"}},
		Protected: []DeleteGroup{{Resource: "invoices", Total: 0}},
	}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cascades) != 1 || got.Cascades[0].Total != 2 || got.Cascades[0].More != 0 {
		t.Errorf("cascades %+v", got.Cascades)
	}
	if len(got.Protected) != 0 || got.Blocked {
		t.Errorf("an empty protected group must not block: %+v", got)
	}
}

func TestHostLabelWinsAndUnmanagedTypesRenderAsText(t *testing.T) {
	source := &previewSource{}
	admin := newPreviewAdmin(source)
	source.preview = DeletePreview{Cascades: []DeleteGroup{
		{Resource: "users", Label: "Members", Objects: items("a")},
		{Label: "Sessions", Objects: items("s1", "s2")},
	}}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Cascades[0].Label != "Members" {
		t.Errorf("label = %q", got.Cascades[0].Label)
	}
	s := got.Cascades[1]
	if s.ModelAdmin != nil || !reflect.DeepEqual(s.Texts, []string{"item s1", "item s2"}) || len(s.Visible) != 0 {
		t.Errorf("unmanaged group %+v", s)
	}
}

func TestGroupsNeedAResourceOrALabelAndAKnownSlug(t *testing.T) {
	for name, group := range map[string]DeleteGroup{
		"neither":      {Objects: items("a")},
		"unknown slug": {Resource: "ghosts", Objects: items("a")},
	} {
		source := &previewSource{}
		admin := newPreviewAdmin(source)
		source.preview = DeletePreview{Cascades: []DeleteGroup{group}}
		if _, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme")); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestHostErrorPropagates(t *testing.T) {
	source := &previewSource{err: errors.New("db down")}
	admin := newPreviewAdmin(source)
	if _, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme")); err == nil {
		t.Error("the host's error was swallowed")
	}
}

func TestRecordsThePrincipalCannotViewAreCountedNotNamed(t *testing.T) {
	source := &previewSource{}
	deny := previewAuthorizer(func(permission string, resource any) bool {
		item, ok := resource.(previewItem)
		return !(permission == "users.view" && ok && item.Name == "secret")
	})
	admin := newPreviewAdmin(source, WithAuthorizer(deny))
	source.preview = DeletePreview{Cascades: []DeleteGroup{{Resource: "users", Objects: items("open", "secret")}}}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	g := got.Cascades[0]
	if len(g.Visible) != 1 || g.Visible[0].(previewItem).Name != "open" || g.Hidden != 1 {
		t.Errorf("got %+v", g)
	}
}

func TestProtectedRecordsBlock(t *testing.T) {
	source := &previewSource{}
	admin := newPreviewAdmin(source)
	source.preview = DeletePreview{Protected: []DeleteGroup{{Resource: "invoices", Objects: items("inv")}}}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Blocked || len(got.Protected) != 1 || len(got.DeniedTypes) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestCascadeIntoATypeThePrincipalCannotDeleteBlocks(t *testing.T) {
	source := &previewSource{}
	deny := previewAuthorizer(func(permission string, resource any) bool { return permission != "users.delete" })
	admin := newPreviewAdmin(source, WithAuthorizer(deny))
	source.preview = DeletePreview{Cascades: []DeleteGroup{
		{Resource: "users", Objects: items("a")},
		{Label: "Sessions", Objects: items("s")}, // unmanaged types never block on permission
	}}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Blocked || !reflect.DeepEqual(got.DeniedTypes, []string{"User"}) {
		t.Errorf("got %+v", got)
	}
}

func TestCascadeIntoATypeWithDeleteDisabledBlocks(t *testing.T) {
	source := &previewSource{}
	readOnly := newPreviewTarget("Ledger", "ledgers")
	readOnly.DisableDelete = true
	source.BaseModelAdmin = BaseModelAdmin{ModelName: "Organization", SlugOverride: "organizations"}
	admin := New(WithModelAdmins(source, readOnly))
	source.preview = DeletePreview{Cascades: []DeleteGroup{{Resource: "ledgers", Objects: items("l")}}}
	got, err := ResolveDeletePreview(context.Background(), admin, source, nil, items("acme"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Blocked || !reflect.DeepEqual(got.DeniedTypes, []string{"Ledger"}) {
		t.Errorf("got %+v", got)
	}
}

func TestSelectionFingerprintIsSortedAndStable(t *testing.T) {
	target := newPreviewTarget("User", "users")
	objs := []any{previewItem{ID: 2}, previewItem{ID: 10}, previewItem{ID: 1}}
	// sha256("1\n10\n2"): byte-order sort, newline-joined. The Python
	// implementation asserts the same literal.
	const want = "32d5c2d95a5bef8b0f13c335628b73422fa80b6eba491f32520f57a395c464bb"
	if got := SelectionFingerprint(target, objs); got != want {
		t.Errorf("got %s", got)
	}
}
