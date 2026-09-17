package core

import "time"

// asTime reads a field value that is a time, in either the value or the
// pointer form. Used by DateFilter (core/filter.go).
func asTime(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case *time.Time:
		if v == nil {
			return time.Time{}, false
		}
		return *v, true
	}
	return time.Time{}, false
}
