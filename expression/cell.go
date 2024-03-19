package expression

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var identifierPattern = regexp.MustCompile("(?P<column>[A-Z]+)(?P<row>[0-9]+)")

func CellCoordinates(in string) (int, int, error) {
	in = strings.TrimPrefix(in, "cell-")
	if !identifierPattern.MatchString(in) {
		return 0, 0, fmt.Errorf("unexpected identifier pattern expected something like A4")
	}
	parts := identifierPattern.FindStringSubmatch(in)
	columnName := parts[identifierPattern.SubexpIndex("column")]
	row, err := strconv.Atoi(parts[identifierPattern.SubexpIndex("row")])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to Parse row number: %w", err)
	}
	//if row > maxRow {
	//	return 0, 0, fmt.Errorf("row number %d out of range it must be greater than 0 and less than or equal to %d", row, maxRow)
	//}
	column := columnNumber(columnName)
	//if column > maxColumn {
	//	return 0, 0, fmt.Errorf("column %s out of range it must be greater than or equal to %s and less than or equal to %s", columnName, columnLabel(0), columnLabel(maxColumn))
	//}
	return column, row, nil
}

func columnNumber(label string) int {
	result := 0
	for _, char := range label {
		result = result*26 + int(char) - 64
	}
	return result - 1
}
