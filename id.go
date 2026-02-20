package clice

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

type ID struct {
	Column, Row int
}

const columnCharacterCount = int('Z') - int('A') + 1

var identifierPattern = regexp.MustCompile("^(?P<column>[A-Z]+)(?P<row>[0-9]+)$")

func ParseID(in string) (ID, error) {
	return parseID([]byte(in))
}

func parseID(in []byte) (ID, error) {
	in = bytes.TrimPrefix(in, []byte("cell-"))
	if !identifierPattern.Match(in) {
		return ID{}, fmt.Errorf("unexpected identifier pattern expected something like A4")
	}
	parts := identifierPattern.FindSubmatch(in)
	columnName := parts[identifierPattern.SubexpIndex("column")]
	row, err := strconv.ParseInt(string(parts[identifierPattern.SubexpIndex("row")]), 10, 64)
	if err != nil {
		return ID{}, fmt.Errorf("failed to Parse row number: %w", err)
	}
	column := columnNumber(columnName)
	return ID{Column: column, Row: int(row)}, nil
}

func (cid ID) appendBytes(buf []byte) []byte {
	cl := digitsInBase(cid.Column, columnCharacterCount)
	rl := digitsInBase(cid.Row, 10)
	if cap(buf) == 0 {
		buf = make([]byte, 0, cl+rl)
	} else {
		buf = slices.Grow(buf, cl+rl)
	}
	return strconv.AppendInt(columnLabel(buf, cid.Column), int64(cid.Row), 10)
}

func (cid ID) AppendText(buf []byte) ([]byte, error) {
	return cid.appendBytes(buf), nil
}
func (cid ID) String() string               { return string(cid.appendBytes(nil)) }
func (cid ID) ElementID() string            { return "cell-" + string(cid.appendBytes(nil)) }
func (cid ID) MarshalText() ([]byte, error) { return cid.appendBytes(nil), nil }

func (cid *ID) UnmarshalText(text []byte) error {
	var err error
	*cid, err = parseID(text)
	return err
}

func columnNumber(label []byte) int {
	result := 0
	for _, char := range label {
		result = result*26 + int(char) - 64
	}
	return result - 1
}

func columnLabel(buf []byte, n int) []byte {
	initialN := len(buf)
	buf = slices.Grow(buf, digitsInBase(n, columnCharacterCount))
	for n >= 0 {
		remainder := n % columnCharacterCount
		buf = append(buf, uint8(remainder+int('A')))
		n = n/columnCharacterCount - 1
	}
	slices.Reverse(buf[initialN:])
	return buf
}

func logBaseN(x, n int) int {
	if x < n {
		return 0
	}

	result := 0
	for x >= n {
		x /= n
		result++
	}
	return result
}

func digitsInBase(x, n int) int {
	if x == 0 {
		return 1
	}
	return logBaseN(x, n) + 1
}

func powInt(n, exp int) int {
	out := n
	for range exp {
		out *= n
	}
	return out
}
