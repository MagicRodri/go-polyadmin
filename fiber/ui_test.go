package fiber

import (
	"regexp"
	"strings"
	"testing"
)

func TestUIClassesFillsInBothDefaultAxes(t *testing.T) {
	// Neither axis given: the shadcn defaults for variant *and* size
	// should both appear.
	got, err := uiClasses("button")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	for _, want := range []string{
		"inline-flex",    // base
		"bg-primary",     // variant "default"
		"h-10 px-4 py-2", // size "size-default"
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestUIClassesExplicitVariantSuppressesDefaultVariant(t *testing.T) {
	got, err := uiClasses("button", "outline")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(got, "border border-input") {
		t.Errorf("expected the outline variant, got %q", got)
	}
	// The default variant's fill must not leak in alongside it --
	// two competing bg-* utilities would resolve by Tailwind's own
	// output order rather than intent.
	if strings.Contains(got, "bg-primary text-primary-foreground") {
		t.Errorf("default variant leaked into an explicit one: %q", got)
	}
	// Size was not given, so it still defaults.
	if !strings.Contains(got, "h-10") {
		t.Errorf("expected the default size, got %q", got)
	}
}

func TestUIClassesExplicitSizeSuppressesDefaultSize(t *testing.T) {
	got, err := uiClasses("button", "ghost", "size-icon-sm")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(got, "h-8 w-8") {
		t.Errorf("expected size-icon-sm, got %q", got)
	}
	if strings.Contains(got, "h-10 px-4") {
		t.Errorf("default size leaked into an explicit one: %q", got)
	}
}

func TestUIClassesRejectsUnknownComponentAndModifier(t *testing.T) {
	// A typo has to fail loudly: a template func returning an error
	// aborts ExecuteTemplate, so this surfaces in the render tests
	// rather than shipping an unstyled control.
	if _, err := uiClasses("buton"); err == nil {
		t.Error("expected an error for an unknown component")
	}
	if _, err := uiClasses("button", "outlined"); err == nil {
		t.Error("expected an error for an unknown modifier")
	}
}

func TestUIClassesHasNoStrayWhitespace(t *testing.T) {
	// Most "size-default" entries are deliberately empty, so the join
	// has to drop them rather than emit a double space.
	got, err := uiClasses("input")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if strings.Contains(got, "  ") || got != strings.TrimSpace(got) {
		t.Errorf("stray whitespace in %q", got)
	}
}

func TestUIRegistryStructuralInvariants(t *testing.T) {
	for component, spec := range uiRegistry {
		// Something has to be resolvable, or the entry is dead weight.
		if spec.Base == "" && len(spec.Parts) == 0 {
			t.Errorf("component %q has neither a base nor any parts", component)
		}
		// A size axis is only usable if it can default.
		if len(spec.Sizes) > 0 {
			if _, ok := spec.Sizes["size-default"]; !ok {
				t.Errorf("component %q has sizes but no \"size-default\" to fall back on", component)
			}
		}
		// The "size-" prefix is how the resolver tells the axes apart.
		for name := range spec.Sizes {
			if !strings.HasPrefix(name, "size-") {
				t.Errorf("component %q size %q must be prefixed \"size-\"", component, name)
			}
		}
		for name := range spec.Variants {
			if strings.HasPrefix(name, "size-") {
				t.Errorf("component %q variant %q must not be prefixed \"size-\"", component, name)
			}
		}
		for name := range spec.Parts {
			if strings.HasPrefix(name, "size-") {
				t.Errorf("component %q part %q must not be prefixed \"size-\"", component, name)
			}
		}
		// One name, one meaning.
		for name := range spec.Parts {
			if _, clash := spec.Variants[name]; clash {
				t.Errorf("component %q declares %q as both a part and a variant", component, name)
			}
		}
	}
}

// tailwindProperty maps a utility to the CSS property it sets, for the
// conflict check below. Only the properties that actually collide in
// this registry are listed -- a base fighting its own variants over
// height, padding, or background is the realistic failure.
func tailwindProperty(class string) string {
	// Strip responsive/state prefixes (sm:, hover:, focus-visible:, ...);
	// a prefixed utility only applies conditionally, so it never
	// unconditionally fights an unprefixed one.
	if strings.Contains(class, ":") {
		return ""
	}
	switch {
	case strings.HasPrefix(class, "h-"):
		return "height"
	case strings.HasPrefix(class, "w-"):
		return "width"
	case strings.HasPrefix(class, "bg-"):
		return "background"
	case strings.HasPrefix(class, "px-"):
		return "padding-x"
	case strings.HasPrefix(class, "py-"):
		return "padding-y"
	case strings.HasPrefix(class, "p-"):
		return "padding"
	case strings.HasPrefix(class, "text-") && !strings.HasPrefix(class, "text-left") && !strings.HasPrefix(class, "text-right") && !strings.HasPrefix(class, "text-center"):
		return "" // text-* is both color and size; too ambiguous to check
	default:
		return ""
	}
}

func propertiesOf(classes string) map[string]string {
	out := map[string]string{}
	for _, class := range strings.Fields(classes) {
		if prop := tailwindProperty(class); prop != "" {
			out[prop] = class
		}
	}
	return out
}

func TestUIRegistryBaseDoesNotFightItsVariants(t *testing.T) {
	// There is no tailwind-merge here (see ui.go), so a base that sets a
	// property its own variant or size also sets produces two competing
	// utilities in one class attribute -- and Tailwind resolves those by
	// its own output order, not the order written. shadcn's cva entries
	// avoid this by construction; so must these.
	for component, spec := range uiRegistry {
		baseProps := propertiesOf(spec.Base)
		for _, group := range []struct {
			kind    string
			entries map[string]string
		}{
			{"variant", spec.Variants},
			{"size", spec.Sizes},
		} {
			for name, classes := range group.entries {
				for prop, class := range propertiesOf(classes) {
					if baseClass, clash := baseProps[prop]; clash {
						t.Errorf("ui(%q).Base sets %s via %q, but %s %q also sets it via %q -- move it out of the base",
							component, prop, baseClass, group.kind, name, class)
					}
				}
			}
		}
	}
}

func TestUIRegistryPartsUsableFromClassList(t *testing.T) {
	// Parts are handed to a DOM classList in a few places (the
	// combobox's arrow-key handler is the load-bearing one), and
	// classList.add throws InvalidCharacterError on a value containing a
	// space. Only the parts that are actually used that way must be
	// single classes, so this checks the known one explicitly rather than
	// constraining every part.
	got, err := uiClasses("combobox", "item-active")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if strings.ContainsAny(got, " \t\n") {
		t.Errorf("combobox item-active is passed to classList.add and must be one class, got %q", got)
	}
}

func TestUIClassesRejectsPartCombinedWithOtherModifiers(t *testing.T) {
	// A part replaces the base, so composing it with a variant is
	// meaningless -- better to say so than to silently pick one.
	if _, err := uiClasses("card", "title", "outline"); err == nil {
		t.Error("expected an error when a part is combined with another modifier")
	}
}

func TestUIClassesPartResolvesWithoutTheBase(t *testing.T) {
	// The bug this structure exists to prevent: `table`'s base is the
	// <table> element's own classes, and a <th> must not inherit them.
	th, err := uiClasses("table", "th")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if strings.Contains(th, "caption-bottom") || strings.Contains(th, "w-full") {
		t.Errorf("table th leaked the table base: %q", th)
	}
}

func TestUIRegistryUsesThemeTokensNotLiteralPalette(t *testing.T) {
	// The whole point of the port: colors resolve through the CSS
	// variables in admin/theme.html. A literal neutral-*/gray-* here
	// would be invisible to the theme and to dark mode.
	//
	// The emerald/amber pairs are the documented exceptions -- there is
	// no shadcn success/warning token to defer to, so those name an
	// explicit dark: variant instead.
	//
	// Anchored on a preceding word boundary and a trailing shade number,
	// so this matches `bg-slate-50` but not `translate-x-4` -- an
	// unanchored substring search flags the latter.
	banned := regexp.MustCompile(`(?:^|[\s:])(?:[a-z-]+-)?(?:neutral|gray|slate|zinc|stone)-\d+|\bbg-white\b|\btext-black\b`)
	check := func(component, modifier, classes string) {
		if match := banned.FindString(classes); match != "" {
			t.Errorf("ui(%q, %q) uses the literal palette %q: %s", component, modifier, strings.TrimSpace(match), classes)
		}
	}
	for component, spec := range uiRegistry {
		check(component, "base", spec.Base)
		for _, group := range []map[string]string{spec.Variants, spec.Sizes, spec.Parts} {
			for modifier, classes := range group {
				check(component, modifier, classes)
			}
		}
	}
}

func TestUIRegistryMatchesThePythonImplementationKeyForKey(t *testing.T) {
	// The two registries are maintained as mirrors (see ui.go's header),
	// so this pins the shape a reader comparing them relies on. Kept as
	// an explicit list rather than parsed from the Python source -- the
	// point is to fail when someone adds a component to one side only.
	// The mirror of this test lives in
	// python-polyadmin/tests/test_ui.py.
	expected := map[string]bool{
		// Phase A
		"button": true, "input": true, "textarea": true, "select": true,
		"label": true, "checkbox": true, "radio": true, "switch": true,
		"badge": true, "card": true, "alert": true, "separator": true,
		"skeleton": true, "avatar": true, "text": true,
		// Phase B
		"dialog": true, "dropdown": true, "popover": true, "tooltip": true,
		"toast": true, "sheet": true,
		// Phase C
		"sidebar": true, "nav-item": true, "tabs": true, "accordion": true,
		"breadcrumb": true, "pagination": true, "table": true,
		// Phase D
		"field": true, "combobox": true, "calendar": true, "slider": true,
		"multi-select": true,
		// dashboard / misc
		"widget": true, "panel": true, "page": true, "toolbar": true,
		"filter-panel": true,
	}
	for component := range uiRegistry {
		if !expected[component] {
			t.Errorf("component %q is in the Go registry but not the expected set (add it to the Python mirror too)", component)
		}
	}
	for component := range expected {
		if _, ok := uiRegistry[component]; !ok {
			t.Errorf("component %q is expected but missing from the Go registry", component)
		}
	}
}

func TestDictAndListBuildPartialArguments(t *testing.T) {
	got, err := dictValues("label", "Export", "items", listValues(1, 2))
	if err != nil {
		t.Fatalf("dictValues: %v", err)
	}
	if got["label"] != "Export" {
		t.Errorf("got %v", got)
	}
	if items, ok := got["items"].([]any); !ok || len(items) != 2 {
		t.Errorf("expected a 2-element list, got %v", got["items"])
	}
	if _, err := dictValues("odd"); err == nil {
		t.Error("expected an error for an odd argument count")
	}
	if _, err := dictValues(1, "value"); err == nil {
		t.Error("expected an error for a non-string key")
	}
}
