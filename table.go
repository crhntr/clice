package clice

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/crhntr/clice/expression"
)

type Cell struct {
	row    int
	column int

	expression ast.Expr
	value      constant.Value

	expressionInput string

	err error
}

func (cell *Cell) Column() int {
	return cell.column
}

func (cell *Cell) Row() int {
	return cell.row
}

func (cell *Cell) Expression() string {
	if cell.expression != nil && cell.err == nil {
		s, err := expression.String(cell.expression)
		if err != nil {
			return cell.expressionInput
		}
		return s
	}
	return cell.expressionInput
}

func (cell *Cell) String() string {
	if cell.value == nil {
		return ""
	}
	return cell.value.String()
}

func (cell *Cell) Error() string {
	if cell.err == nil {
		return ""
	}
	return cell.err.Error()
}

func (cell *Cell) HasExpression() bool {
	return cell.expression != nil
}

type EncodedCell struct {
	ID         string `json:"id"`
	Expression string `json:"ex"`
}

func (cell *Cell) MarshalJSON() ([]byte, error) {
	s, err := expression.String(cell.expression)
	if err != nil {
		s = cell.expressionInput
	}
	return json.Marshal(EncodedCell{
		ID:         strings.TrimPrefix(cell.ID(), "cell-"),
		Expression: s,
	})
}

type EncodedTable struct {
	ColumnCount int           `json:"columns"`
	RowCount    int           `json:"rows"`
	Cells       []EncodedCell `json:"cells"`
}

func (table *Table) UnmarshalJSON(in []byte) error {
	var encoded EncodedTable

	if err := json.Unmarshal(in, &encoded); err != nil {
		return err
	}
	table.RowCount = encoded.RowCount
	table.ColumnCount = encoded.ColumnCount
	for _, cell := range encoded.Cells {
		column, row, err := CellID(cell.ID)
		if err != nil {
			return err
		}
		exp, err := expression.New(cell.Expression)
		if err != nil {
			return err
		}
		table.Cells = append(table.Cells, Cell{
			column:     column,
			row:        row,
			expression: exp,
		})
	}

	return table.Evaluate()
}

func (cell *Cell) ID() string {
	return fmt.Sprintf("%s%d", columnLabel(cell.column), cell.row)
}

type Table struct {
	ColumnCount int    `json:"columns"`
	RowCount    int    `json:"rows"`
	Cells       []Cell `json:"cells"`
}

func NewTable(columns, rows int) Table {
	table := Table{
		RowCount:    rows,
		ColumnCount: columns,
	}
	return table
}

func (table *Table) Rows() []Row {
	result := make([]Row, table.RowCount)
	for i := range result {
		result[i].Number = i
	}
	return result
}

func (table *Table) Columns() []Column {
	result := make([]Column, table.ColumnCount)
	for i := range result {
		result[i].Number = i
	}
	return result
}

func (table *Table) Evaluate() error {
	cells := slices.Clone(table.Cells)
	for i := range cells {
		visited := make(visitSet)
		cell := &cells[i]
		err := cell.evaluate(table, visited)
		if err != nil {
			cell.err = err
			return err
		}
		cell.err = nil
	}
	slices.SortFunc(cells, func(c1, c2 Cell) int {
		if c1.column == c2.column {
			return c1.row - c2.row
		}
		return c1.column - c2.column
	})
	table.Cells = cells
	return nil
}

func (table *Table) Cell(column, row int) *Cell {
	for i, cell := range table.Cells {
		if cell.row == row && cell.column == column {
			return &table.Cells[i]
		}
	}
	table.Cells = append(table.Cells, Cell{
		row:    row,
		column: column,
	})
	return &table.Cells[len(table.Cells)-1]
}

type visit struct {
	colum, row int
}

type visitSet map[visit]struct{}

func (set visitSet) check(v visit) bool {
	_, visited := set[v]
	set[v] = struct{}{}
	return visited
}

func (cell *Cell) evaluate(table *Table, visited visitSet) error {
	v := visit{
		colum: cell.column,
		row:   cell.row,
	}
	_, alreadyVisited := visited[v]
	if alreadyVisited {
		return fmt.Errorf("recursive reference to %s%d", columnLabel(cell.column), cell.row)
	}
	visited[v] = struct{}{}
	if cell.expression == nil {
		cell.value = constant.MakeInt64(0)
		return nil
	}
	result, err := expression.Evaluate(newScope(table, cell), cell.expression)
	if err != nil {
		return err
	}
	cell.value = result
	return nil
}

type Scope struct {
	Table   *Table
	cell    *Cell
	visited visitSet
}

func newScope(table *Table, cell *Cell) *Scope {
	return &Scope{
		Table:   table,
		cell:    cell,
		visited: make(visitSet),
	}
}

func (s *Scope) Resolve(ident string) (constant.Value, error) {
	switch ident {
	case "iota":
		return constant.MakeInt64(int64(s.cell.row)), nil
	default:
		if !identifierPattern.MatchString(ident) {
			return nil, fmt.Errorf("unknown variable %s", ident)
		}
		column, row, err := CellID(ident)
		if err != nil {
			return nil, err
		}
		if s.visited.check(visit{row: row, colum: column}) {
			return nil, fmt.Errorf("recursive reference to %s%d", columnLabel(column), row)
		}
		cell := s.Table.Cell(column, row)
		if cell.expression == nil {
			return constant.MakeInt64(0), nil
		}
		return expression.Evaluate(&Scope{
			cell:    cell,
			Table:   s.Table,
			visited: s.visited,
		}, cell.expression)
	}
}

var identifierPattern = regexp.MustCompile("(?P<column>[A-Z]+)(?P<row>[0-9]+)")

func CellID(in string) (int, int, error) {
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

type Column struct {
	Number int
}

func (column Column) Label() string {
	return columnLabel(column.Number)
}

func columnLabel(n int) string {
	result := ""
	for n >= 0 {
		remainder := n % 26
		result = fmt.Sprintf("%c", remainder+65) + result
		n = n/26 - 1
	}
	return result
}

type Row struct {
	Number int
}

func (row Row) Label() string {
	return strconv.Itoa(row.Number)
}

type Assignment struct {
	Identifier string
	Expression string
}

func (table *Table) Apply(assignments ...Assignment) error {
	var parseError error
	for _, assignment := range assignments {
		column, row, err := CellID(assignment.Identifier)
		if err != nil {
			return err
		}
		cell := table.Cell(column, row)
		exp, err := expression.New(assignment.Expression)
		if err != nil {
			return fmt.Errorf("failed to parse %s expression %s: %w", assignment.Identifier, assignment.Expression, err)
		}
		cell.expressionInput = assignment.Expression
		cell.expression = exp
		cell.value = nil
		cell.err = nil
	}
	if parseError != nil {
		return fmt.Errorf("failed to parse some expressions: %w", parseError)
	}
	return table.Evaluate()
}
