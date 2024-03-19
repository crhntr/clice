package main

import (
	"go/constant"
	"strconv"
	"testing"

	"github.com/crhntr/clice/expression"
)

func Test_parse(t *testing.T) {
	for _, tt := range []struct {
		Name       string
		Expression string
		Result     int
	}{
		{
			Name:       "just 1",
			Expression: "1",
			Result:     1,
		},
		{
			Name:       "add",
			Expression: "1 + 2",
			Result:     3,
		},
		{
			Name:       "subtract",
			Expression: "1 - 2",
			Result:     -1,
		},
		{
			Name:       "multiply",
			Expression: "2 * 3",
			Result:     6,
		},
		{
			Name:       "divide",
			Expression: "6 / 2",
			Result:     3,
		},
		{
			Name:       "no space",
			Expression: "8/2",
			Result:     4,
		},
		{
			Name:       "space around",
			Expression: " 8/2 ",
			Result:     4,
		},
		{
			Name:       "cell in cells slice",
			Expression: "A1",
			Result:     100,
		},
		{
			Name:       "cell not in cells slice",
			Expression: "J9",
			Result:     0,
		},
		{
			Name:       "multiple multiply expressions",
			Expression: "1 * 2 * 3",
			Result:     6,
		},
		{
			Name:       "precedence order",
			Expression: "1 * 2 + 3",
			Result:     5,
		},
		{
			Name:       "non precedence order",
			Expression: "1 + 2 * 3",
			Result:     7,
		},
		{
			Name:       "non precedence order on both sides",
			Expression: "1 + 2 * 3 + 4",
			Result:     11,
		},
		{
			Name:       "number in parens",
			Expression: "(1)",
			Result:     1,
		},
		{
			Name:       "two sets of parens in middle",
			Expression: "(1 + 2) * (3 + 4)",
			Result:     21,
		},
		{
			Name:       "one set of parens with binary op with higher president",
			Expression: "2 * (3 + 4)",
			Result:     14,
		},
		{
			Name:       "division has higher president over subtraction",
			Expression: "100 - 6 / 3",
			Result:     98,
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			table := NewTable(10, 10)
			const otherVal = 100
			e, err := expression.New(strconv.Itoa(otherVal))
			if err != nil {
				t.Fatal(err)
			}
			table.Cells = []Cell{{Column: 0, Row: 1, Value: constant.MakeInt64(otherVal), Expression: e}}

			node, err := expression.New(tt.Expression)
			if err != nil {
				t.Fatal(err)
			}

			value, err := expression.Evaluate(newScope(&table, &Cell{Column: 0, Row: 0}), node)
			if err != nil {
				t.Fatal(err)
			}

			if value.String() != strconv.Itoa(tt.Result) {
				t.Errorf("expected %d but got %d", tt.Result, value)
			}
		})
	}
}
