package clice

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"slices"
	"strconv"

	"github.com/crhntr/clice/expression"
)

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
	table.RowLen = encoded.RowCount
	table.ColumnLen = encoded.ColumnCount
	for _, cell := range encoded.Cells {
		exp, err := expression.New(cell.Expression)
		if err != nil {
			return err
		}
		table.Cells = append(table.Cells, Cell{
			column:     cell.ID.Column,
			row:        cell.ID.Row,
			expression: exp,
		})
	}

	return table.Evaluate()
}

type Table struct {
	ColumnLen int    `json:"columns"`
	RowLen    int    `json:"rows"`
	Cells     []Cell `json:"cells"`
}

func NewTable(columns, rows int) Table {
	table := Table{
		RowLen:    rows,
		ColumnLen: columns,
	}
	return table
}

func (table *Table) Rows() []Row {
	result := make([]Row, table.RowLen)
	for i := range result {
		result[i].Number = i
	}
	return result
}

func (table *Table) Columns() []Column {
	result := make([]Column, table.ColumnLen)
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
		cid, err := ParseID(ident)
		if err != nil {
			return nil, err
		}
		if cid.Row < 0 || cid.Row >= s.Table.RowLen {
			return nil, fmt.Errorf("row index %d out of bounds [0, %d)", cid.Row, s.Table.RowLen)
		}
		if cid.Column < 0 || cid.Column >= s.Table.ColumnLen {
			return nil, fmt.Errorf("column index %d out of bounds [0, %d)", cid.Column, s.Table.ColumnLen)
		}
		if s.visited.check(visit{row: cid.Row, colum: cid.Column}) {
			return nil, fmt.Errorf("recursive reference to %s", ident)
		}
		cell := s.Table.Cell(cid.Column, cid.Row)
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

type Column struct {
	Number int
}

func (column Column) Label() string {
	return string(columnLabel(nil, column.Number))
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
	for _, assignment := range assignments {
		cid, err := ParseID(assignment.Identifier)
		if err != nil {
			return err
		}
		cell := table.Cell(cid.Column, cid.Row)
		exp, err := expression.New(assignment.Expression)
		if err != nil {
			return fmt.Errorf("failed to parse %s expression %s: %w", assignment.Identifier, assignment.Expression, err)
		}
		cell.expressionInput = assignment.Expression
		cell.expression = exp
		cell.value = nil
		cell.err = nil
	}
	return table.Evaluate()
}
