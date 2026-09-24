package datasync

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Decode decodes entity JSON into a new model of table. It accepts the Go
// model JSON (snake_case) as well as Java entity JSON: camelCase keys and the
// nested "upk" composite key are flattened, and Java-only aliases are mapped
// (votePartitionID -> vote_participant_id, desc -> description ...).
func (r *Registry) Decode(table string, data []byte) (interface{}, error) {
	obj, ok := r.New(table)
	if !ok {
		return nil, fmt.Errorf("table %q is not synchronizable", table)
	}
	if len(data) == 0 || string(data) == "null" {
		return nil, fmt.Errorf("empty data for table %q", table)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode %s: %w", table, err)
	}
	norm := normalizeKeys(raw)
	coerce(reflect.TypeOf(obj).Elem(), norm)
	b, err := json.Marshal(norm)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, obj); err != nil {
		return nil, fmt.Errorf("decode %s: %w", table, err)
	}
	return obj, nil
}

// keyAliases maps Java field names (after snake_case conversion) to Go JSON keys.
var keyAliases = map[string]string{
	"vote_partition_i_d": "vote_participant_id",
	"vote_partition_id":  "vote_participant_id",
	"vote_i_d":           "vote_id",
	"desc":               "description",
}

func normalizeKeys(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	put := func(k string, v interface{}) {
		sk := toSnake(k)
		if a, ok := keyAliases[sk]; ok {
			sk = a
		}
		// Snake-case keys already present win over converted camelCase twins.
		if _, exists := out[sk]; exists && sk != k {
			return
		}
		out[sk] = v
	}
	// First pass: keys already in snake_case.
	for k, v := range in {
		if k == "upk" {
			continue
		}
		if k == toSnake(k) {
			put(k, v)
		}
	}
	for k, v := range in {
		if k == "upk" {
			continue
		}
		if k != toSnake(k) {
			put(k, v)
		}
	}
	if upk, ok := in["upk"].(map[string]interface{}); ok {
		for k, v := range upk {
			put(k, v)
		}
	}
	return out
}

// toSnake converts camelCase to snake_case ("nodeId" -> "node_id",
// "gmtCreate" -> "gmt_create"); snake_case input is returned unchanged.
func toSnake(s string) string {
	if !strings.ContainsFunc(s, unicode.IsUpper) {
		return s
	}
	var b strings.Builder
	runes := []rune(s)
	for i, c := range runes {
		if unicode.IsUpper(c) {
			if i > 0 && runes[i-1] != '_' {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(c))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

var timeType = reflect.TypeOf(time.Time{})

// jsonFields maps JSON keys to field types (embedded structs flattened).
func jsonFields(t reflect.Type, out map[string]reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			jsonFields(f.Type, out)
			continue
		}
		if !f.IsExported() {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

// ParseTime parses the time formats exchanged by Java/Go nodes (RFC3339,
// LocalDateTime ISO, "yyyy-MM-dd HH:mm:ss"); zone-less values are local time.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// coerce fixes JSON values that do not fit the Go field type: non-string
// values for string fields are stored as their JSON text (Java lists/sets),
// Java LocalDateTime strings/arrays for time fields are converted, numeric
// strings for number fields are parsed. Unconvertible values are dropped.
func coerce(t reflect.Type, m map[string]interface{}) {
	fields := map[string]reflect.Type{}
	jsonFields(t, fields)
	for k, v := range m {
		ft, ok := fields[k]
		if !ok {
			delete(m, k)
			continue
		}
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if v == nil {
			continue
		}
		switch {
		case ft == timeType:
			switch tv := v.(type) {
			case string:
				if ts, ok := ParseTime(tv); ok {
					m[k] = ts.Format(time.RFC3339Nano)
				} else {
					delete(m, k)
				}
			case []interface{}: // Jackson LocalDateTime array [y,M,d,h,m,s,n]
				parts := make([]int, 7)
				for i := 0; i < len(tv) && i < 7; i++ {
					if f, ok := tv[i].(float64); ok {
						parts[i] = int(f)
					}
				}
				if len(tv) >= 3 {
					m[k] = time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], parts[6], time.Local).Format(time.RFC3339Nano)
				} else {
					delete(m, k)
				}
			case map[string]interface{}: // gorm.DeletedAt {Time, Valid}
			default:
				delete(m, k)
			}
		case ft.Kind() == reflect.String:
			switch sv := v.(type) {
			case string:
			case float64:
				m[k] = strconv.FormatFloat(sv, 'f', -1, 64)
			case bool:
				m[k] = strconv.FormatBool(sv)
			default:
				b, err := json.Marshal(sv)
				if err != nil {
					delete(m, k)
				} else {
					m[k] = string(b)
				}
			}
		case ft.Kind() >= reflect.Int && ft.Kind() <= reflect.Float64:
			switch sv := v.(type) {
			case float64:
			case bool:
				if sv {
					m[k] = 1
				} else {
					m[k] = 0
				}
			case string:
				if f, err := strconv.ParseFloat(sv, 64); err == nil {
					m[k] = f
				} else {
					delete(m, k)
				}
			default:
				delete(m, k)
			}
		case ft.Kind() == reflect.Bool:
			switch sv := v.(type) {
			case bool:
			case float64:
				m[k] = sv != 0
			case string:
				b, err := strconv.ParseBool(sv)
				if err != nil {
					delete(m, k)
				} else {
					m[k] = b
				}
			default:
				delete(m, k)
			}
		}
	}
}
