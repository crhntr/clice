package clice

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"

	"github.com/crhntr/clice/expression"
)

type Cell struct {
	row    int
	column int

	expression ast.Expr
	value      fmt.Stringer

	expressionInput string

	err error
}

func (cell *Cell) Column() int {
	return cell.column
}

func (cell *Cell) Row() int {
	return cell.row
}

func (cell *Cell) ID() string { return (&ID{Column: cell.column, Row: cell.row}).String() }

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
	ID         ID     `json:"id"`
	Expression string `json:"ex"`
}

func (cell *Cell) MarshalJSON() ([]byte, error) {
	s, err := expression.String(cell.expression)
	if err != nil {
		s = cell.expressionInput
	}
	return json.Marshal(EncodedCell{
		ID:         ID{Column: cell.column, Row: cell.row},
		Expression: s,
	})
}

func (cell *Cell) evaluate(table *Table, visited visitSet) error {
	v := visit{
		colum: cell.column,
		row:   cell.row,
	}
	_, alreadyVisited := visited[v]
	if alreadyVisited {
		return fmt.Errorf("recursive reference to %s", cell.ID())
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
