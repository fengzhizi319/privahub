package service

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/scritchley/orc"
)

// orcMagic is the ORC file header ("ORC"). SecretFlow >= 1.x writes
// component output tables as ORC (dist_data dataRefs[].format = "orc"),
// while data/download promises a CSV file.
var orcMagic = []byte("ORC")

// isORCFile reports whether path starts with the ORC magic bytes.
func isORCFile(path string) bool {
	f, err := os.Open(path) //nolint:gosec // G304: path is contained by DataService.Download (inDir + EvalSymlinks)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(orcMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return bytes.Equal(head, orcMagic)
}

// recoverORC turns a panic of the ORC decoder (it panics on some corrupt
// inputs instead of returning an error) into an error.
func recoverORC(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("orc decoder panic: %v", r)
	}
}

// checkORCReadable opens the ORC footer so a corrupt / unsupported file is
// detected before the HTTP headers are sent.
func checkORCReadable(path string) (err error) {
	defer recoverORC(&err)
	r, err := orc.Open(path)
	if err != nil {
		return err
	}
	return r.Close()
}

// writeORCAsCSV streams an ORC file as CSV (header row = ORC column names).
func writeORCAsCSV(w io.Writer, path string) (err error) {
	defer recoverORC(&err)
	r, err := orc.Open(path)
	if err != nil {
		return fmt.Errorf("open orc: %w", err)
	}
	defer r.Close()
	cols := r.Schema().Columns()
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	c := r.Select(cols...)
	rec := make([]string, len(cols))
	for c.Stripes() {
		for c.Next() {
			row := c.Row()
			for i := range rec {
				rec[i] = ""
				if i < len(row) {
					rec[i] = orcValueString(row[i])
				}
			}
			if err := cw.Write(rec); err != nil {
				return err
			}
		}
	}
	if err := c.Err(); err != nil {
		return fmt.Errorf("read orc: %w", err)
	}
	cw.Flush()
	return cw.Error()
}

// orcValueString formats one ORC cell like pandas.to_csv: null -> "",
// floats without exponent noise, booleans as True/False.
func orcValueString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case bool:
		if x {
			return "True"
		}
		return "False"
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case time.Time:
		return x.Format("2006-01-02 15:04:05.999999999")
	case orc.Date:
		return x.Format("2006-01-02")
	case *big.Int:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}
