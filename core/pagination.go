package core

// Page is an in-memory page over an already-materialized slice. Once a
// real ORM/repository integration exists, it'll typically paginate at
// the query level and only use Page as the result shape.
type Page struct {
	Items      []any
	Number     int
	PageSize   int
	TotalCount int
}

func (p Page) NumPages() int {
	if p.PageSize <= 0 || p.TotalCount <= 0 {
		return 1
	}
	pages := p.TotalCount / p.PageSize
	if p.TotalCount%p.PageSize != 0 {
		pages++
	}
	return pages
}

func (p Page) HasPrevious() bool { return p.Number > 1 }
func (p Page) HasNext() bool     { return p.Number < p.NumPages() }

func (p Page) PreviousPage() int {
	if p.HasPrevious() {
		return p.Number - 1
	}
	return 0
}

func (p Page) NextPage() int {
	if p.HasNext() {
		return p.Number + 1
	}
	return 0
}

// PageOf builds the result shape around an already-windowed slice --
// for a data source that did its own LIMIT/OFFSET and reported the
// total separately. Paginate is the in-memory equivalent, which slices
// and counts for you.
func PageOf(items []any, total int, req ListRequest) Page {
	page, pageSize := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}
	if req.Unlimited {
		// One page holding everything, so NumPages/HasNext stay honest.
		page, pageSize = 1, max(total, 1)
	}
	return Page{Items: items, Number: page, PageSize: pageSize, TotalCount: total}
}

func Paginate(items []any, page, pageSize int) Page {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 1
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return Page{Items: items[start:end], Number: page, PageSize: pageSize, TotalCount: total}
}
