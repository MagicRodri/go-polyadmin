package fiber

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	texttemplate "text/template"
	"text/template/parse"

	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/MagicRodri/go-polyadmin/locales"
	coretemplates "github.com/MagicRodri/go-polyadmin/templates"

	"github.com/gofiber/fiber/v2"
)

// requiredPluralForms are the CLDR forms each shipped language needs.
var requiredPluralForms = map[string][]string{
	"fr": {"one", "other"},
	"ru": {"one", "few", "many", "other"},
}

// msgids maps each extracted msgid to its English plural ("" when the
// string is not a plural).
type msgids map[string]string

func (m msgids) add(singular, plural string) {
	if _, ok := m[singular]; !ok || plural != "" {
		m[singular] = plural
	}
}

// templateMsgids collects literal arguments to t, tn and tjs from every
// embedded template.
func templateMsgids(t *testing.T, out msgids) {
	err := fs.WalkDir(coretemplates.FS, "admin", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		src, err := fs.ReadFile(coretemplates.FS, p)
		if err != nil {
			return err
		}
		// Parsed through text/template (not the lower-level parse.Parse)
		// so builtins like printf, used by real templates, are defined;
		// parse.Parse alone only knows the funcs handed to it.
		tmpl, err := texttemplate.New(p).Funcs(templateFuncs).Parse(string(src))
		if err != nil {
			return err
		}
		for _, tree := range tmpl.Templates() {
			if tree.Tree == nil {
				continue
			}
			walkTemplate(tree.Tree.Root, func(cmd *parse.CommandNode) {
				if len(cmd.Args) < 2 {
					return
				}
				ident, ok := cmd.Args[0].(*parse.IdentifierNode)
				if !ok {
					return
				}
				first, ok := cmd.Args[1].(*parse.StringNode)
				if !ok {
					return
				}
				switch ident.Ident {
				case "t", "tjs":
					out.add(first.Text, "")
				case "tn":
					if len(cmd.Args) > 2 {
						if second, ok := cmd.Args[2].(*parse.StringNode); ok {
							out.add(first.Text, second.Text)
						}
					}
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func walkTemplate(node parse.Node, visit func(*parse.CommandNode)) {
	switch n := node.(type) {
	case *parse.ListNode:
		if n != nil {
			for _, child := range n.Nodes {
				walkTemplate(child, visit)
			}
		}
	case *parse.ActionNode:
		walkTemplate(n.Pipe, visit)
	case *parse.PipeNode:
		if n != nil {
			for _, cmd := range n.Cmds {
				walkTemplate(cmd, visit)
			}
		}
	case *parse.CommandNode:
		visit(n)
		for _, arg := range n.Args {
			walkTemplate(arg, visit)
		}
	case *parse.IfNode:
		walkBranch(&n.BranchNode, visit)
	case *parse.RangeNode:
		walkBranch(&n.BranchNode, visit)
	case *parse.WithNode:
		walkBranch(&n.BranchNode, visit)
	case *parse.TemplateNode:
		walkTemplate(n.Pipe, visit)
	}
}

func walkBranch(n *parse.BranchNode, visit func(*parse.CommandNode)) {
	walkTemplate(n.Pipe, visit)
	walkTemplate(n.List, visit)
	walkTemplate(n.ElseList, visit)
}

// codeCalls maps a translating function's name to the positions of its
// msgid arguments: singular first, plural second.
var codeCalls = map[string][]int{
	"T": {1}, "TN": {1, 2}, "N_": {0},
	"tr": {1}, "trn": {1, 2},
	"t": {0}, "tn": {0, 1},
	"Translate": {1}, "TranslatePlural": {1, 2},
}

// codeMsgids collects literal msgid arguments from the non-test Go files of
// core and fiber.
func codeMsgids(t *testing.T, out msgids) {
	for _, dir := range []string{"../core", "."} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(parsed, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var name string
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				positions, ok := codeCalls[name]
				if !ok {
					return true
				}
				literal := func(i int) (string, bool) {
					if i >= len(call.Args) {
						return "", false
					}
					lit, ok := call.Args[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return "", false
					}
					s, err := strconv.Unquote(lit.Value)
					return s, err == nil
				}
				singular, ok := literal(positions[0])
				if !ok {
					return true
				}
				plural := ""
				if len(positions) > 1 {
					plural, _ = literal(positions[1])
				}
				out.add(singular, plural)
				return true
			})
		}
	}
}

func TestCatalogsAreComplete(t *testing.T) {
	used := msgids{}
	templateMsgids(t, used)
	codeMsgids(t, used)
	if len(used) < 50 {
		t.Fatalf("extracted only %d msgids -- the extractor is broken", len(used))
	}

	for locale, forms := range requiredPluralForms {
		raw, err := fs.ReadFile(locales.FS, locale+".json")
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]json.RawMessage
		if err := json.Unmarshal(raw, &catalog); err != nil {
			t.Fatalf("%s.json: %v", locale, err)
		}

		missing := map[string]any{}
		for singular, plural := range used {
			entry, ok := catalog[singular]
			if !ok {
				if plural == "" {
					missing[singular] = ""
				} else {
					missing[singular] = map[string]string{"one": "", "other": ""}
				}
				continue
			}
			if plural == "" {
				var s string
				if json.Unmarshal(entry, &s) != nil || s == "" {
					t.Errorf("%s: %q needs a non-empty string", locale, singular)
				}
				continue
			}
			var got map[string]string
			if json.Unmarshal(entry, &got) != nil {
				t.Errorf("%s: %q is a plural and needs an object of forms", locale, singular)
				continue
			}
			for _, form := range forms {
				if got[form] == "" {
					t.Errorf("%s: %q lacks plural form %q", locale, singular, form)
				}
			}
		}
		var unused []string
		for id := range catalog {
			if _, ok := used[id]; !ok {
				unused = append(unused, id)
			}
		}
		sort.Strings(unused)
		if len(unused) > 0 {
			t.Errorf("%s: entries nothing uses (delete them): %q", locale, unused)
		}
		if len(missing) > 0 {
			snippet, _ := json.MarshalIndent(missing, "", "  ")
			t.Errorf("%s: %d msgids missing -- add translations for:\n%s", locale, len(missing), snippet)
			if dir := os.Getenv("POLYADMIN_WRITE_MISSING"); dir != "" {
				_ = os.WriteFile(filepath.Join(dir, locale+".missing.json"), snippet, 0o644)
			}
		}
	}
}

// doLocalePostForm POSTs form with the admin_locale cookie set to locale.
// Like doPseudoPostForm, the locale and CSRF cookies are combined into one
// Cookie header up front: a caller-supplied header would otherwise
// overwrite rather than join the one doPostForm sets for the CSRF pair.
func doLocalePostForm(t *testing.T, app *fiber.App, path, locale string, form url.Values) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := core.NewCSRFToken()
	req.Header.Set("Cookie", core.CSRFCookieName+"="+token+"; "+localeCookieName+"="+locale)
	req.Header.Set(core.CSRFHeaderName, token)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

// TestLocaleRenderingHasNoFormatErrors is a sanity check that a
// French/Russian page renders without fmt errors -- a translation whose
// placeholders don't match the
// msgid's (wrong verb, wrong count, dropped %[n]s index) would otherwise
// only show up as "%!s(MISSING)" et al. buried in HTML, easy to miss by
// eye. It reuses the pseudo-locale sweep's page list (pseudo_test.go) so
// every converted area gets checked, but only greps for "%!" rather than
// asserting full translation coverage the way the pseudo sweep does.
func TestLocaleRenderingHasNoFormatErrors(t *testing.T) {
	for _, locale := range []string{"fr", "ru"} {
		for _, p := range sweepPages {
			t.Run(locale+"/"+p.area+"/"+p.name, func(t *testing.T) {
				if !sweepAreas[p.area] {
					t.Skipf("area %q not converted yet", p.area)
				}
				app, _ := p.app(t)
				var resp *http.Response
				if p.method == "POST" {
					resp = doLocalePostForm(t, app, p.path, locale, p.form)
				} else {
					headers := map[string]string{"Cookie": localeCookieName + "=" + locale}
					for k, v := range p.headers {
						headers[k] = v
					}
					resp = doGet(t, app, p.path, headers)
				}
				if resp.StatusCode >= 500 {
					t.Fatalf("got %d", resp.StatusCode)
				}
				if page := body(t, resp); strings.Contains(page, "%!") {
					t.Errorf("fmt error rendering %s in %s:\n%s", p.path, locale, page)
				}
			})
		}
	}
}

// fmtVerb matches one fmt directive: an optional explicit argument index,
// flags, width and precision, then the verb. "%%" matches too and is
// skipped by fmtVerbs, being a literal percent sign rather than a verb.
var fmtVerb = regexp.MustCompile(`%(?:\[(\d+)\])?[-+# 0]*\d*(?:\.\d*)?([a-zA-Z%])`)

// fmtVerbs is the multiset of s's fmt verbs, each keyed by the argument
// it formats: "1:s", "2:d". An explicit index %[n]s names argument n, and
// the directives after it continue from n+1, as fmt does -- so a
// translation may reorder its arguments with %[2]s ... %[1]s and still
// match the msgid's %s ... %s.
func fmtVerbs(s string) map[string]int {
	out := map[string]int{}
	arg := 1
	for _, m := range fmtVerb.FindAllStringSubmatch(s, -1) {
		if m[2] == "%" {
			continue
		}
		if m[1] != "" {
			arg, _ = strconv.Atoi(m[1])
		}
		out[strconv.Itoa(arg)+":"+m[2]]++
		arg++
	}
	return out
}

// verbMismatches compares a catalog entry's translations with the msgid
// they translate: a plain entry with the msgid; a plural entry's "one"
// form with the singular msgid and every other form with the plural. A
// translation that drops, adds or retypes an argument formats as
// "%!d(MISSING)" or "%!(EXTRA ...)" at runtime.
func verbMismatches(singular, plural string, entry json.RawMessage) []string {
	var problems []string
	check := func(form, msgid, text string) {
		if want, got := fmtVerbs(msgid), fmtVerbs(text); !maps.Equal(want, got) {
			problems = append(problems, fmt.Sprintf("%q [%s] %q: verbs %v, want %v", singular, form, text, got, want))
		}
	}
	var text string
	if json.Unmarshal(entry, &text) == nil {
		check("", singular, text)
		return problems
	}
	var forms map[string]string
	if json.Unmarshal(entry, &forms) != nil {
		return []string{fmt.Sprintf("%q: neither a string nor plural forms", singular)}
	}
	if plural == "" {
		plural = singular
	}
	for form, text := range forms {
		if form == "one" {
			check(form, singular, text)
		} else {
			check(form, plural, text)
		}
	}
	return problems
}

func TestCatalogTranslationsKeepEveryVerb(t *testing.T) {
	used := msgids{}
	templateMsgids(t, used)
	codeMsgids(t, used)
	for locale := range requiredPluralForms {
		raw, err := fs.ReadFile(locales.FS, locale+".json")
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]json.RawMessage
		if err := json.Unmarshal(raw, &catalog); err != nil {
			t.Fatalf("%s.json: %v", locale, err)
		}
		for msgid, entry := range catalog {
			for _, problem := range verbMismatches(msgid, used[msgid], entry) {
				t.Errorf("%s: %s", locale, problem)
			}
		}
	}
}

// Proof the parity check bites, on hand-built entries.
func TestVerbParityCheckerCatchesBadEntries(t *testing.T) {
	bad := map[string]struct{ singular, plural, entry string }{
		"dropped verb":     {"Deleted %d record.", "Deleted %d records.", `{"one": "Un enregistrement supprimé.", "other": "%d enregistrements supprimés."}`},
		"retyped verb":     {"%s applied", "", `"%d appliqué"`},
		"extra verb":       {"Save", "", `"Enregistrer %s"`},
		"wrong index":      {"%s of %d", "", `"%[1]s sur %[1]d"`},
		"plural form lost": {"%d row", "%d rows", `{"one": "%d ligne", "other": "lignes"}`},
	}
	for name, c := range bad {
		if len(verbMismatches(c.singular, c.plural, json.RawMessage(c.entry))) == 0 {
			t.Errorf("%s: not flagged", name)
		}
	}
	good := map[string]struct{ singular, plural, entry string }{
		"same verbs":      {"%s of %d", "", `"%s sur %d"`},
		"reordered":       {"%s of %d", "", `"%[2]d : %[1]s"`},
		"literal percent": {"100%% done", "", `"100 %% fait"`},
		"plural":          {"%d row", "%d rows", `{"one": "%d ligne", "other": "%d lignes"}`},
	}
	for name, c := range good {
		if problems := verbMismatches(c.singular, c.plural, json.RawMessage(c.entry)); len(problems) != 0 {
			t.Errorf("%s: wrongly flagged: %v", name, problems)
		}
	}
}
