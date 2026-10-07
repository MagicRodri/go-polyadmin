package fiber

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"slices"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// The page between choosing a form action and running it
// (docs/model-admin.md). It reuses delete_selected's selection plumbing --
// hidden pks or select-all filters, _return, _fingerprint, _confirmed -- so
// a form action works from the list's bulk bar and a detail page alike.

const changePrefix = "_change_"

type actionFormRequest struct {
	admin      *core.Admin
	modelAdmin core.ModelAdmin
	renderer   *Renderer
	principal  *core.Principal
	action     core.Action
	objects    []any
	selectAll  bool
	returnTo   string
	// ticked is the rows a bulk edit's user ticked "Change" on.
	ticked map[string]bool
}

type actionFormView struct {
	Selection deleteSelection
	Fields    []core.Field
	Data      map[string]any
	Errors    map[string][]string
	Relations map[string]*relationFieldOptions
	Ticked    map[string]bool
	Blocked   []string
}

type actionFormRow struct {
	Name   string
	Input  template.HTML
	Ticked bool
}

type actionFormData struct {
	pageBase
	Heading        string
	ActionName     string
	SubmitLabel    string
	CountText      string
	Items          []deleteItemView
	More           int
	Selection      deleteSelection
	Rows           []actionFormRow
	NonFieldErrors []string
	Blocked        []string
	BulkEdit       bool
}

func isSingleRelation(fieldType core.FieldType) bool {
	return fieldType == core.FieldTypeForeignKey || fieldType == core.FieldTypeOneToOne
}

// pkString compares pks as strings: the posted one is a string, and a
// target's GetPK need not be.
func pkString(value any) string {
	if core.IsNil(value) {
		return ""
	}
	return stringOrEmpty(value)
}

func defaultActionData(fields []core.Field) map[string]any {
	data := map[string]any{}
	for _, field := range fields {
		if field.HasDefault {
			data[field.Name] = field.Default
		}
	}
	return data
}

func parseActionForm(c *fiber.Ctx, fields []core.Field, only map[string]bool) map[string]any {
	data := make(map[string]any, len(fields))
	for _, field := range fields {
		if only != nil && !only[field.Name] {
			continue
		}
		switch field.Type {
		case core.FieldTypeBoolean:
			data[field.Name] = c.FormValue(field.Name) != ""
		case core.FieldTypeManyToMany:
			raw := c.Context().PostArgs().PeekMulti(field.Name)
			values := make([]string, len(raw))
			for i, v := range raw {
				values[i] = string(v)
			}
			data[field.Name] = values
		default:
			data[field.Name] = field.ParseFormValue(formValue(c, field.Name))
		}
	}
	return data
}

func validateActionForm(ctx context.Context, fields []core.Field, data map[string]any) map[string][]string {
	errs := map[string][]string{}
	for _, field := range fields {
		value, ok := data[field.Name]
		if !ok {
			continue
		}
		if fieldErrs := field.Validate(ctx, value); len(fieldErrs) > 0 {
			errs[field.Name] = fieldErrs
		}
	}
	return errs
}

// actionAutocompleteNames is which relation fields render as the lookup
// combobox. Bulk edit follows the ModelAdmin's own edit form; any other
// action's single-valued relation does whenever its target can be searched.
func actionAutocompleteNames(admin *core.Admin, modelAdmin core.ModelAdmin, action core.Action) map[string]bool {
	names := map[string]bool{}
	if action.Name == core.BulkEditName {
		for _, name := range modelAdmin.AutocompleteFields() {
			names[name] = true
		}
		return names
	}
	for _, field := range action.Form {
		if field.Relation == nil || !isSingleRelation(field.Type) {
			continue
		}
		if target, ok := admin.GetModelAdmin(field.Relation.Target); ok && len(target.SearchFields()) > 0 {
			names[field.Name] = true
		}
	}
	return names
}

// actionRelationOptions is computeRelationOptions for fields that belong to
// no record: the current value is a posted pk, not a related object.
//
// Gated on the principal's "{target}.view", the check the target's /lookup
// makes: an action may need no more than the resource's own view, so the
// coarser CanView the edit form relies on would name records the principal
// may not see.
func actionRelationOptions(ctx context.Context, admin *core.Admin, principal *core.Principal, fields []core.Field, data map[string]any, autocomplete map[string]bool) map[string]*relationFieldOptions {
	result := map[string]*relationFieldOptions{}
	for _, field := range fields {
		if field.Relation == nil {
			continue
		}
		targetAdmin, ok := admin.GetModelAdmin(field.Relation.Target)
		if !ok || !targetAdmin.CanView() {
			continue
		}
		if admin.Authorizer != nil && !admin.Authorizer.Can(principal, core.ResourcePermission(field.Relation.Target, "view"), targetAdmin) {
			continue
		}
		displayField, _ := targetAdmin.Field(field.Relation.DisplayField)
		value := data[field.Name]
		if autocomplete[field.Name] && isSingleRelation(field.Type) {
			opts := &relationFieldOptions{Autocomplete: true, LookupTarget: field.Relation.Target}
			if selected := pkString(value); selected != "" {
				opts.SelectedPK = selected
				if current, err := targetAdmin.GetObject(ctx, selected); err == nil && !core.IsNil(current) {
					opts.SelectedLabel = fmtValue(displayField.GetValue(current))
				}
			}
			result[field.Name] = opts
			continue
		}
		items, _, _ := core.ListObjects(ctx, targetAdmin, core.ListRequest{Unlimited: true})
		options := make([]relationOption, 0, len(items))
		for _, related := range items {
			options = append(options, relationOption{PK: fmt.Sprint(targetAdmin.GetPK(related)), Label: fmtValue(displayField.GetValue(related))})
		}
		opts := &relationFieldOptions{Options: options}
		if field.Type == core.FieldTypeManyToMany {
			values, _ := value.([]string)
			for _, v := range values {
				opts.SelectedPKs = append(opts.SelectedPKs, v)
			}
		} else if selected := pkString(value); selected != "" {
			opts.SelectedPK = selected
		}
		result[field.Name] = opts
	}
	return result
}

// bulkEditBlocked labels the records the principal may not update, or on
// which one of names is read-only. Any at all blocks the whole edit.
func bulkEditBlocked(admin *core.Admin, principal *core.Principal, modelAdmin core.ModelAdmin, objects []any, names map[string]bool) []string {
	permission := core.ResourcePermission(modelAdmin.Slug(), "update")
	var labels []string
	for _, obj := range objects {
		blocked := !authorizeObject(admin, principal, permission, obj)
		for name := range names {
			if modelAdmin.IsReadOnly(name, obj) {
				blocked = true
			}
		}
		if blocked {
			labels = append(labels, objectLabel(modelAdmin, obj))
		}
	}
	return labels
}

func renderActionFormPage(c *fiber.Ctx, req *actionFormRequest, data map[string]any, errs map[string][]string, blocked []string, changed bool, status int) error {
	sel := deleteSelection{
		Objects:     req.objects,
		SelectAll:   req.selectAll,
		Fingerprint: core.SelectionFingerprint(req.modelAdmin, req.objects),
		Return:      req.returnTo,
		Changed:     changed,
	}
	if req.selectAll {
		sel.List = parseListRequestFromForm(c)
	} else {
		for _, obj := range req.objects {
			sel.PKs = append(sel.PKs, fmt.Sprint(req.modelAdmin.GetPK(obj)))
		}
	}
	view := actionFormView{
		Selection: sel,
		Fields:    req.action.Form,
		Data:      data,
		Errors:    errs,
		Relations: actionRelationOptions(c.Context(), req.admin, req.principal, req.action.Form, data, actionAutocompleteNames(req.admin, req.modelAdmin, req.action)),
		Ticked:    req.ticked,
		Blocked:   blocked,
	}
	html, err := req.renderer.RenderActionForm(req.principal, csrfToken(c), req.modelAdmin, req.action, view)
	if err != nil {
		return err
	}
	c.Status(status)
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.SendString(html)
}

// resolveActionForm answers with the form page (done is true), or returns
// the validated data to run the action with.
func resolveActionForm(c *fiber.Ctx, req *actionFormRequest) (bool, map[string]any, error) {
	fields := req.action.Form
	if c.FormValue(confirmedField) == "" {
		return true, nil, renderActionFormPage(c, req, defaultActionData(fields), nil, nil, false, fiber.StatusOK)
	}
	bulk := req.action.Name == core.BulkEditName
	var only map[string]bool
	if bulk {
		only = map[string]bool{}
		for _, field := range fields {
			if c.FormValue(changePrefix+field.Name) != "" {
				only[field.Name] = true
			}
		}
		req.ticked = only
	}
	data := parseActionForm(c, fields, only)
	if req.selectAll && formValue(c, fingerprintField) != core.SelectionFingerprint(req.modelAdmin, req.objects) {
		return true, nil, renderActionFormPage(c, req, data, nil, nil, true, fiber.StatusOK)
	}
	if bulk {
		if len(only) == 0 {
			errs := map[string][]string{"": {tr(c, "Choose at least one field to change.")}}
			return true, nil, renderActionFormPage(c, req, data, errs, nil, false, fiber.StatusUnprocessableEntity)
		}
		if blocked := bulkEditBlocked(req.admin, req.principal, req.modelAdmin, req.objects, only); len(blocked) > 0 {
			return true, nil, renderActionFormPage(c, req, data, nil, blocked, false, fiber.StatusUnprocessableEntity)
		}
	}
	errs := validateActionForm(c.Context(), fields, data)
	if bulk {
		// The edit form's own validation, which a host may have extended.
		// Complaints about unticked fields are dropped: they are not the
		// form's to send, exactly as for read-only ones on the edit page.
		for name, messages := range req.modelAdmin.Validate(c.Context(), data) {
			if !only[name] && name != "" {
				continue
			}
			for _, message := range messages {
				if !slices.Contains(errs[name], message) {
					errs[name] = append(errs[name], message)
				}
			}
		}
	}
	if len(errs) > 0 {
		return true, nil, renderActionFormPage(c, req, data, errs, nil, false, fiber.StatusUnprocessableEntity)
	}
	return false, data, nil
}

func (r *Renderer) RenderActionForm(principal *core.Principal, csrfToken string, modelAdmin core.ModelAdmin, action core.Action, view actionFormView) (string, error) {
	count := len(view.Selection.Objects)
	label := r.t(action.Label)
	submit := action.SubmitLabel
	if submit == "" {
		submit = action.Label
	}
	crumbs := append(r.categoryBreadcrumb(modelAdmin.Category()),
		breadcrumb{Label: r.t(modelAdmin.VerboseName()), URL: fmt.Sprintf("%s/%s", r.basePath, modelAdmin.Slug())},
		breadcrumb{Label: label, Active: true})
	data := actionFormData{
		pageBase:       r.pageBase(principal, csrfToken, label, label, "resource:"+modelAdmin.Slug(), modelAdmin, crumbs, nil),
		Heading:        label,
		ActionName:     action.Name,
		SubmitLabel:    r.t(submit),
		CountText:      r.tn("%d record selected", "%d records selected", count, count),
		Selection:      view.Selection,
		NonFieldErrors: view.Errors[""],
		Blocked:        view.Blocked,
		BulkEdit:       action.Name == core.BulkEditName,
	}
	data.Items, data.More = r.selectionSample(principal, modelAdmin, view.Selection.Objects)
	for _, field := range view.Fields {
		value, ok := view.Data[field.Name]
		if !ok && field.HasDefault {
			value = field.Default
		}
		input, err := r.formInputHTML(r.basePath, field, value, view.Errors[field.Name], view.Relations[field.Name], false)
		if err != nil {
			return "", err
		}
		data.Rows = append(data.Rows, actionFormRow{Name: field.Name, Input: input, Ticked: view.Ticked[field.Name]})
	}
	tmpl, err := r.contentTemplate(modelAdmin, "action_form", r.actionFormTpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
