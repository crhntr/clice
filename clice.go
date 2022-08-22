package clice

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"reflect"
)

type Cell struct {
	Column, Row int

	ss *SpreadSheet

	Expression           string
	observers, observing map[*Cell]struct{}

	value

	view interface {
		SetTextContent(s string)
	}
}

func newCell(ss *SpreadSheet, column, row int) *Cell {
	return &Cell{
		ss:        ss,
		Column:    column,
		Row:       row,
		observing: make(map[*Cell]struct{}),
		observers: make(map[*Cell]struct{}),
		value:     reflectValue(reflect.ValueOf(0)),
	}
}

func (cell *Cell) update(ctx context.Context) error {
	return cell.ss.UpdateCell(ctx, cell.Column, cell.Row, cell.Expression)
}
func (cell *Cell) attach(_ context.Context, u *Cell) {
	cell.observers[u] = struct{}{}
	u.observing[cell] = struct{}{}
}
func (cell *Cell) detach(_ context.Context, u *Cell) {
	delete(cell.observers, u)
	delete(u.observing, cell)
}
func (cell *Cell) references(ctx context.Context, u *Cell) bool {
	if cell == u {
		return true
	}
	for c := range cell.observing {
		if c.references(ctx, u) {
			return true
		}
	}
	return false
}
func (cell *Cell) Interface() any       { return cell.Value().Interface() }
func (cell *Cell) Value() reflect.Value { return cell.value.Value() }

func (cell *Cell) String() string {
	return fmt.Sprintf("Cell{Column: %d, Row: %d, Exp: %q, Type: %q, Value: %#v}\n",
		cell.Column, cell.Row, cell.Expression, cell.Value().Type(), cell.Interface(),
	)
}

func (cell *Cell) set(ctx context.Context, v value) error {
	previous := cell.value
	cell.value = v
	for o := range cell.observers {
		err := o.update(ctx)
		if err != nil {
			cell.value = previous
			return err
		}
	}
	return nil
}

type SpreadSheet struct {
	uiElQuery string
	Cells     []*Cell

	Columns, Rows int

	scope map[string]kinder
}

type kinder interface {
	Kind() reflect.Kind
}

func New() *SpreadSheet {
	scope := make(map[string]kinder)
	builtinTypes(scope)
	ss := &SpreadSheet{
		Columns: 32,
		Rows:    128,
		scope:   scope,
	}
	return ss
}

func (ss *SpreadSheet) FindCell(_ context.Context, column, row int) (*Cell, bool) {
	index := ss.indexOf(column, row)
	if index < 0 {
		return nil, false
	}
	return ss.Cells[index], true
}

func (ss *SpreadSheet) save(_ context.Context, cell *Cell) {
	index := ss.indexOf(cell.Column, cell.Row)
	if index < 0 {
		ss.Cells = append(ss.Cells, cell)
		return
	}
	ss.Cells[index] = cell
}

func (ss *SpreadSheet) indexOf(column, row int) int {
	for i, c := range ss.Cells {
		if c.Column == column && c.Row == row {
			return i
		}
	}
	return -1
}

func (ss *SpreadSheet) UpdateCell(ctx context.Context, column, row int, expression string) error {
	if row >= ss.Rows || row < 0 {
		return fmt.Errorf("row out of range of spreadsheet")
	}
	if column >= ss.Columns || column < 0 {
		return fmt.Errorf("column out of range of spreadsheet")
	}

	var cell *Cell

	existingCell, isExistingCell := ss.FindCell(ctx, column, row)
	if isExistingCell {
		cell = existingCell
		if expression != cell.Expression {
			for ref := range cell.observing {
				ref.detach(ctx, cell)
			}
		}
	} else {
		cell = newCell(ss, column, row)
	}

	exp, err := parser.ParseExpr(expression)
	if err != nil {
		return err
	}

	val, err := ss.evaluate(ctx, cell, exp)
	if err != nil {
		return err
	}

	err = cell.set(ctx, val)
	if err != nil {
		return err
	}
	cell.Expression = expression
	ss.save(ctx, cell)
	if cell.view != nil {
		cell.view.SetTextContent(fmt.Sprintf("%v", cell.Value().Interface()))
	}
	return nil
}

func (ss *SpreadSheet) AddFunction(name string, v interface{}) error {
	fn := reflect.ValueOf(v)
	if fn.Kind() != reflect.Func {
		return fmt.Errorf("v must be a function")
	}

	if !ast.IsExported(name) {
		return fmt.Errorf("function name must be exported")
	}

	ss.scope[name] = fn

	return nil
}

func (ss *SpreadSheet) lookUp(_ context.Context, name string) (kinder, error) {
	v, found := ss.scope[name]
	if !found {
		return nil, errUnknownIdentifier(name)
	}
	return v, nil
}
