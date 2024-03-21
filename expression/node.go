package expression

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/printer"
	"go/token"
)

type Node = ast.Expr

type Scope interface {
	Resolve(string) (constant.Value, error)
}

func New(in string) (ast.Expr, error) {
	if in == "" {
		return nil, nil
	}
	return parser.ParseExpr(in)
}

func String(expr ast.Expr) (string, error) {
	if expr == nil {
		return "", nil
	}
	set := token.NewFileSet()
	var buf bytes.Buffer
	err := printer.Fprint(&buf, set, expr)
	return buf.String(), err
}

func Evaluate(scope Scope, expr ast.Expr) (_ constant.Value, err error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		cv := constant.MakeFromLiteral(e.Value, e.Kind, 0)
		return cv, nil
	case *ast.UnaryExpr:
		x, err := Evaluate(scope, e.X)
		if err != nil {
			return nil, err
		}
		return constant.UnaryOp(e.Op, x, 0), nil
	case *ast.BinaryExpr:
		left, err := Evaluate(scope, e.X)
		if err != nil {
			return nil, err
		}
		right, err := Evaluate(scope, e.Y)
		if err != nil {
			return nil, err
		}
		return constant.BinaryOp(left, e.Op, right), nil
	case *ast.ParenExpr:
		return Evaluate(scope, e.X)
	case *ast.Ident:
		switch e.Name {
		case "true":
			return constant.MakeBool(true), nil
		case "false":
			return constant.MakeBool(false), nil
		default:
			return scope.Resolve(e.Name)
		}
	default:
		return nil, &UnsupportedError{Expr: expr}
	}
}

type UnsupportedError struct {
	ast.Expr
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("unsupported expression type: %T", e.Expr)
}
