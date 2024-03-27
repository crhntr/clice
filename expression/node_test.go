package expression_test

import (
	"errors"
	"fmt"
	"go/constant"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crhntr/clice/expression"
)

func TestEvaluate_Integers(t *testing.T) {
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
			Name:       "unary operator",
			Expression: "-A1",
			Result:     -100,
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
			node, err := expression.New(tt.Expression)
			if err != nil {
				t.Fatal(err)
			}

			scope := fakeScopeFunc(func(s string) (constant.Value, error) {
				if s == "A1" {
					return constant.MakeInt64(100), nil
				}
				return constant.MakeInt64(0), nil
			})

			value, err := expression.Evaluate(scope, node)
			if err != nil {
				t.Fatal(err)
			}

			if value.String() != strconv.Itoa(tt.Result) {
				t.Errorf("expected %d but got %d", tt.Result, value)
			}
		})
	}
}

func TestEvaluate_Booleans(t *testing.T) {
	resolveCallCount := 0
	resolve := fakeScopeFunc(func(s string) (constant.Value, error) {
		resolveCallCount++
		switch s {
		case "happy":
			return constant.MakeBool(true), nil
		case "wealthy":
			return constant.MakeBool(false), nil
		default:
			return constant.MakeUnknown(), fmt.Errorf("unexpected cell reference: %s", s)
		}
	})
	resetCallCount := func() {
		resolveCallCount = 0
	}

	t.Run("true", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("true")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "true", value.String())
	})

	t.Run("false", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("false")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "false", value.String())
	})

	t.Run("unary operator", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("!happy")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "false", value.String())
	})

	t.Run("multiple unary operators", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("!!happy")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "true", value.String())
	})

	t.Run("binary operator and", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("happy && wealthy")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "false", value.String())
		assert.Equal(t, 2, resolveCallCount)
	})
	t.Run("binary operator and short circuit", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("wealthy && happy")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "false", value.String())
		assert.Equal(t, 1, resolveCallCount)
	})
	t.Run("binary operator or", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("wealthy || happy")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "true", value.String())
		assert.Equal(t, 2, resolveCallCount)
	})
	t.Run("binary operator or short circuit", func(t *testing.T) {
		t.Cleanup(resetCallCount)
		node, err := expression.New("happy || wealthy")
		require.NoError(t, err)

		value, err := expression.Evaluate(resolve, node)
		require.NoError(t, err)

		assert.Equal(t, constant.Bool, value.Kind())
		assert.Equal(t, "true", value.String())
		assert.Equal(t, 1, resolveCallCount)
	})
}

func TestEvaluate(t *testing.T) {
	t.Run("parsing a negative cell value", func(t *testing.T) {
		node, err := expression.New("-J9")
		if err != nil {
			t.Fatal(err)
		}

		scope := fakeScopeFunc(func(s string) (constant.Value, error) {
			return constant.MakeInt64(0), fmt.Errorf("banana")
		})

		_, err = expression.Evaluate(scope, node)
		assert.ErrorContains(t, err, "banana")
	})

	t.Run("either side of a boolean expression fail", func(t *testing.T) {
		t.Run("left side fails", func(t *testing.T) {
			scope := fakeScopeFunc(func(s string) (constant.Value, error) {
				switch s {
				case "a":
					return constant.MakeInt64(2), nil
				case "b":
					return constant.MakeInt64(0), fmt.Errorf("banana")
				default:
					t.Fatal("unexpected cell reference")
					return nil, nil
				}
			})

			node, err := expression.New("a + b")
			if err != nil {
				t.Fatal(err)
			}

			_, err = expression.Evaluate(scope, node)
			assert.ErrorContains(t, err, "banana")
		})

		t.Run("right side fails", func(t *testing.T) {
			scope := fakeScopeFunc(func(s string) (constant.Value, error) {
				switch s {
				case "a":
					return constant.MakeInt64(0), fmt.Errorf("banana")
				case "b":
					return constant.MakeInt64(2), nil
				default:
					t.Fatal("unexpected cell reference")
					return nil, nil
				}
			})

			node, err := expression.New("a + b")
			if err != nil {
				t.Fatal(err)
			}

			_, err = expression.Evaluate(scope, node)
			assert.ErrorContains(t, err, "banana")
		})
	})

	t.Run("inline function node", func(t *testing.T) {
		node, err := expression.New("func(x int) int { return 1 + x}(ten)")
		if err != nil {
			t.Fatal(err)
		}

		scope := fakeScopeFunc(func(s string) (constant.Value, error) {
			switch s {
			case "ten":
				return constant.MakeInt64(10), nil
			default:
				t.Fatal("unexpected cell reference")
				return nil, nil
			}
		})

		_, err = expression.Evaluate(scope, node)

		var exprErr *expression.UnsupportedError
		require.True(t, errors.As(err, &exprErr))
		require.ErrorContains(t, err, "unsupported expression type: *ast.CallExpr")
	})
}

func TestString(t *testing.T) {
	t.Run("nil expression", func(t *testing.T) {
		s, err := expression.String(nil)
		require.NoError(t, err)
		assert.Equal(t, "", s)
	})

	t.Run("empty expression", func(t *testing.T) {
		node, err := expression.New("")
		require.NoError(t, err)
		s, err := expression.String(node)
		require.NoError(t, err)
		assert.Equal(t, "", s)
	})

	t.Run("simple expression", func(t *testing.T) {
		node, err := expression.New("1+2")
		require.NoError(t, err)

		s, err := expression.String(node)
		require.NoError(t, err)
		assert.Equal(t, "1 + 2", s)
	})
}

type fakeScopeFunc func(string) (constant.Value, error)

func (f fakeScopeFunc) Resolve(s string) (constant.Value, error) {
	return f(s)
}
