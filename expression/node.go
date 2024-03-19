package expression

import (
	"fmt"
	"go/constant"
	"go/token"
)

type Scope interface {
	Resolve(string) (constant.Value, error)
}

type Node interface {
	fmt.Stringer
	Evaluate(s Scope) (constant.Value, error)
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

func (node IdentifierNode) String() string { return node.Token.Value }
func (node IdentifierNode) Evaluate(s Scope) (constant.Value, error) {
	return s.Resolve(node.Token.Value)
}

type ValueNode struct {
	Token Token
	Value constant.Value
}

func (node ValueNode) String() string                         { return node.Token.Value }
func (node ValueNode) Evaluate(Scope) (constant.Value, error) { return node.Value, nil }

type BinaryExpressionNode struct {
	Op          Token
	Left, Right Node
}

func (node BinaryExpressionNode) String() string {
	return fmt.Sprintf("%s %s %s", node.Left.String(), node.Op.Value, node.Right.String())
}

func (node BinaryExpressionNode) Evaluate(s Scope) (constant.Value, error) {
	left, err := node.Left.Evaluate(s)
	if err != nil {
		return nil, err
	}
	right, err := node.Right.Evaluate(s)
	if err != nil {
		return nil, err
	}

	switch node.Op.Type {
	case TokenAdd:
		return constant.BinaryOp(left, token.ADD, right), nil
	case TokenSubtract:
		return constant.BinaryOp(left, token.SUB, right), nil
	case TokenMultiply:
		return constant.BinaryOp(left, token.MUL, right), nil
	case TokenDivide:
		return constant.BinaryOp(left, token.QUO, right), nil
	default:
		return nil, fmt.Errorf("unknown binary operator %s", node.Op.Value)
	}
}

type ParenNode struct {
	Start, End Token
	Node       Node
}

func (node ParenNode) String() string                           { return fmt.Sprintf("(%s)", node.Node) }
func (node ParenNode) Evaluate(s Scope) (constant.Value, error) { return node.Node.Evaluate(s) }
