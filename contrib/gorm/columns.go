package gorm

import (
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
	"gorm.io/gorm/schema"
)

var (
	timeType    = reflect.TypeFor[time.Time]()
	errConvert  = errors.New("value does not convert")
	truthy      = map[string]bool{"true": true, "1": true, "on": true, "yes": true}
	timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02"}
)

func deriveField(f *schema.Field) core.Field {
	typ := core.FieldTypeUnspecified
	t := f.IndirectFieldType
	switch {
	case t == timeType:
		typ = core.FieldTypeDateTime
		if strings.EqualFold(f.TagSettings["TYPE"], "date") {
			typ = core.FieldTypeDate
		}
	case t.Kind() == reflect.Bool:
		typ = core.FieldTypeBoolean
	case isInt(t.Kind()) || isUint(t.Kind()):
		typ = core.FieldTypeInteger
	case isFloat(t.Kind()):
		typ = core.FieldTypeDecimal
	case t.Kind() == reflect.String:
		typ = core.FieldTypeString
	}
	var opts []core.FieldOption
	if typ != core.FieldTypeUnspecified && typ != core.FieldTypeBoolean && isRequired(f) {
		opts = append(opts, core.WithRequired())
	}
	return core.NewField(f.Name, typ, opts...)
}

func isRequired(f *schema.Field) bool {
	notNull := f.NotNull || f.FieldType.Kind() != reflect.Pointer
	return notNull && !f.PrimaryKey && !f.HasDefaultValue && f.AutoCreateTime == 0 && f.AutoUpdateTime == 0
}

// convert turns a submitted or filter value into t, the Go type of a
// column's struct field. nil and "" are the zero value (nil for a pointer).
func convert(value any, t reflect.Type) (reflect.Value, error) {
	if value == nil {
		return reflect.Zero(t), nil
	}
	if s, ok := value.(string); ok && s == "" {
		return reflect.Zero(t), nil
	}
	if v := reflect.ValueOf(value); v.Kind() == reflect.Pointer && v.IsNil() {
		return reflect.Zero(t), nil
	}
	if t.Kind() == reflect.Pointer {
		inner, err := convert(value, t.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(t.Elem())
		ptr.Elem().Set(inner)
		return ptr, nil
	}
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Zero(t), nil
		}
		v = v.Elem()
	}
	switch {
	case v.Type() == t:
		return v, nil
	case v.Kind() == reflect.String:
		return parseString(v.String(), t)
	case isNumber(v.Kind()) && isNumber(t.Kind()):
		return convertNumber(v, t)
	case v.Kind() == reflect.Bool && t.Kind() == reflect.Bool:
		return v.Convert(t), nil
	case v.Type() == timeType && t.ConvertibleTo(timeType) && t.Kind() == reflect.Struct:
		return v.Convert(t), nil
	}
	return reflect.Value{}, errConvert
}

func parseString(s string, t reflect.Type) (reflect.Value, error) {
	s = strings.TrimSpace(s)
	out := reflect.New(t).Elem()
	switch {
	case t == timeType:
		for _, layout := range timeLayouts {
			if parsed, err := time.ParseInLocation(layout, s, time.Local); err == nil {
				return reflect.ValueOf(parsed), nil
			}
		}
		return reflect.Value{}, errConvert
	case t.Kind() == reflect.String:
		out.SetString(s)
	case t.Kind() == reflect.Bool:
		out.SetBool(truthy[strings.ToLower(s)])
	case isInt(t.Kind()):
		n, err := strconv.ParseInt(s, 10, t.Bits())
		if err != nil {
			return reflect.Value{}, errConvert
		}
		out.SetInt(n)
	case isUint(t.Kind()):
		n, err := strconv.ParseUint(s, 10, t.Bits())
		if err != nil {
			return reflect.Value{}, errConvert
		}
		out.SetUint(n)
	case isFloat(t.Kind()):
		n, err := strconv.ParseFloat(s, t.Bits())
		if err != nil {
			return reflect.Value{}, errConvert
		}
		out.SetFloat(n)
	default:
		return reflect.Value{}, errConvert
	}
	return out, nil
}

func convertNumber(v reflect.Value, t reflect.Type) (reflect.Value, error) {
	out := reflect.New(t).Elem()
	var f float64
	switch {
	case isInt(v.Kind()):
		f = float64(v.Int())
	case isUint(v.Kind()):
		f = float64(v.Uint())
	default:
		f = v.Float()
	}
	switch {
	case isInt(t.Kind()):
		if isFloat(v.Kind()) && f != math.Trunc(f) {
			return reflect.Value{}, errConvert
		}
		n := int64(f)
		if isInt(v.Kind()) {
			n = v.Int()
		}
		if isUint(v.Kind()) && v.Uint() > math.MaxInt64 || out.OverflowInt(n) {
			return reflect.Value{}, errConvert
		}
		out.SetInt(n)
	case isUint(t.Kind()):
		if f < 0 || (isFloat(v.Kind()) && f != math.Trunc(f)) {
			return reflect.Value{}, errConvert
		}
		n := uint64(f)
		if isUint(v.Kind()) {
			n = v.Uint()
		} else if isInt(v.Kind()) {
			n = uint64(v.Int())
		}
		if out.OverflowUint(n) {
			return reflect.Value{}, errConvert
		}
		out.SetUint(n)
	default:
		if out.OverflowFloat(f) {
			return reflect.Value{}, errConvert
		}
		out.SetFloat(f)
	}
	return out, nil
}

func isInt(k reflect.Kind) bool {
	return k == reflect.Int || k == reflect.Int8 || k == reflect.Int16 || k == reflect.Int32 || k == reflect.Int64
}

func isUint(k reflect.Kind) bool {
	return k == reflect.Uint || k == reflect.Uint8 || k == reflect.Uint16 || k == reflect.Uint32 || k == reflect.Uint64 || k == reflect.Uintptr
}

func isFloat(k reflect.Kind) bool { return k == reflect.Float32 || k == reflect.Float64 }

func isNumber(k reflect.Kind) bool { return isInt(k) || isUint(k) || isFloat(k) }
