package gorm

import (
	"reflect"
	"testing"
	"time"
)

func TestConvert(t *testing.T) {
	ptr := func(v any) any {
		r := reflect.New(reflect.TypeOf(v))
		r.Elem().Set(reflect.ValueOf(v))
		return r.Interface()
	}
	day := time.Date(2026, 10, 7, 0, 0, 0, 0, time.Local)
	ok := []struct {
		in   any
		to   reflect.Type
		want any
	}{
		{"42", reflect.TypeFor[uint](), uint(42)},
		{42, reflect.TypeFor[int64](), int64(42)},
		{3.0, reflect.TypeFor[int](), 3},
		{"x", reflect.TypeFor[*string](), ptr("x")},
		{"", reflect.TypeFor[*string](), (*string)(nil)},
		{nil, reflect.TypeFor[int](), 0},
		{"on", reflect.TypeFor[bool](), true},
		{"2026-10-07", reflect.TypeFor[*time.Time](), ptr(day)},
		{"1.5", reflect.TypeFor[float64](), 1.5},
	}
	for _, c := range ok {
		got, err := convert(c.in, c.to)
		if err != nil || !reflect.DeepEqual(got.Interface(), c.want) {
			t.Errorf("convert(%#v, %s) = %#v, %v; want %#v", c.in, c.to, got, err, c.want)
		}
	}
	bad := []struct {
		in any
		to reflect.Type
	}{
		{"abc", reflect.TypeFor[int]()},
		{"99999999999999999999", reflect.TypeFor[int64]()},
		{-1, reflect.TypeFor[uint]()},
		{300, reflect.TypeFor[uint8]()},
		{1.5, reflect.TypeFor[int]()},
		{"not a date", reflect.TypeFor[time.Time]()},
	}
	for _, c := range bad {
		if _, err := convert(c.in, c.to); err == nil {
			t.Errorf("convert(%#v, %s) accepted it", c.in, c.to)
		}
	}
}
