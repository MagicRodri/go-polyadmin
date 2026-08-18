package core

import "testing"

func toAnySlice(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func TestPaginateFirstPage(t *testing.T) {
	page := Paginate(toAnySlice(30), 1, 10)
	if len(page.Items) != 10 || page.Items[0] != 0 {
		t.Fatalf("got %v", page.Items)
	}
	if page.NumPages() != 3 {
		t.Fatalf("got %d pages", page.NumPages())
	}
	if page.HasPrevious() {
		t.Fatalf("expected no previous page")
	}
	if !page.HasNext() || page.NextPage() != 2 {
		t.Fatalf("got next=%d", page.NextPage())
	}
}

func TestPaginateLastPage(t *testing.T) {
	page := Paginate(toAnySlice(25), 3, 10)
	if len(page.Items) != 5 {
		t.Fatalf("got %d items", len(page.Items))
	}
	if !page.HasPrevious() || page.HasNext() {
		t.Fatalf("got previous=%v next=%v", page.HasPrevious(), page.HasNext())
	}
}

func TestPaginateEmpty(t *testing.T) {
	page := Paginate([]any{}, 1, 10)
	if len(page.Items) != 0 || page.NumPages() != 1 {
		t.Fatalf("got %v pages=%d", page.Items, page.NumPages())
	}
}

func TestPaginateClampsInvalidInputs(t *testing.T) {
	page := Paginate(toAnySlice(5), 0, 0)
	if page.Number != 1 || page.PageSize != 1 {
		t.Fatalf("got number=%d pageSize=%d", page.Number, page.PageSize)
	}
}
