package expression

import (
	"fmt"
)

type Scope interface {
	Resolve(string) (int, error)
}

type Node interface {
	fmt.Stringer
	Evaluate(s Scope) (int, error)
}

func New(in string) (Node, error) {
	tokens, err := Tokens(in)
	if err != nil {
		return nil, err
	}
	expression, err := Parse(tokens)
	if err != nil {
		return nil, err
	}
	return expression, nil
}

type IdentifierNode struct {
	Token Token
}

func (node IdentifierNode) String() string                { return node.Token.Value }
func (node IdentifierNode) Evaluate(s Scope) (int, error) { return s.Resolve(node.Token.Value) }

type IntegerNode struct {
	Token Token
	Value int
}

func (node IntegerNode) String() string              { return node.Token.Value }
func (node IntegerNode) Evaluate(Scope) (int, error) { return node.Value, nil }

type BinaryExpressionNode struct {
	Op          Token
	Left, Right Node
}

func (node BinaryExpressionNode) String() string {
	return fmt.Sprintf("%s %s %s", node.Left.String(), node.Op.Value, node.Right.String())
}

func (node BinaryExpressionNode) Evaluate(s Scope) (int, error) {
	left, err := node.Left.Evaluate(s)
	if err != nil {
		return 0, err
	}
	right, err := node.Right.Evaluate(s)
	if err != nil {
		return 0, err
	}
	switch node.Op.Type {
	case TokenAdd:
		return left + right, nil
	case TokenSubtract:
		return left - right, nil
	case TokenMultiply:
		return left * right, nil
	case TokenExponent:
		res := 1
		for i := 0; i < right; i++ {
			res *= left
		}
		return res, nil
	case TokenDivide:
		if right == 0 {
			return 0, fmt.Errorf("could not divide by zero")
		}
		return left / right, nil
	default:
		return 0, fmt.Errorf("unknown binary operator %s", node.Op.Value)
	}
}

type FactorialNode struct {
	Expression Node
}

func (node FactorialNode) String() string {
	return fmt.Sprintf("%s!", node.Expression)
}

func (node FactorialNode) Evaluate(scope Scope) (int, error) {
	n, err := node.Expression.Evaluate(scope)
	if err != nil {
		return 0, err
	}
	if n > 20 {
		return 0, fmt.Errorf("n! where n > 20 is too large")
	}
	for i := n - 1; i >= 2; i-- {
		n *= i
	}
	return n, nil
}

type ParenNode struct {
	Start, End Token
	Node       Node
}

func (node ParenNode) String() string                { return fmt.Sprintf("(%s)", node.Node) }
func (node ParenNode) Evaluate(s Scope) (int, error) { return node.Node.Evaluate(s) }
