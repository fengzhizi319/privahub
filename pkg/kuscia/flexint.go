package kuscia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// FlexInt64 is an int64 that decodes from both a JSON number and a JSON
// string. KusciaAPI's HTTP gateway serializes protobuf int64 fields with
// protojson, which emits them as strings ("86400"), while older releases and
// hand-written fixtures use plain numbers. It always encodes as a number,
// which protojson also accepts on input.
type FlexInt64 int64

// UnmarshalJSON implements json.Unmarshaler.
func (f *FlexInt64) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		b = []byte(s)
	}
	v, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return fmt.Errorf("kuscia: invalid int64 %q: %w", b, err)
	}
	*f = FlexInt64(v)
	return nil
}
