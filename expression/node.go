package expression

import (
	"fmt"
)

type Node interface {
	fmt.Stringer
}

func New(in string) (Node, error) {
	expressionText := Normalize(in)
	tokens, err := Tokens(expressionText)
	if err != nil {
		return nil, err
	}
	expression, err := Parse(tokens, 0)
	if err != nil {
		return nil, err
	}
	return expression, nil
}

type IdentifierNode struct {
	Token Token

	Row, Column int
}

func (node IdentifierNode) String() string {
	return node.Token.Value
}

type IntegerNode struct {
	Token Token
	Value int
}

func (node IntegerNode) String() string {
	return node.Token.Value
}

func (node IntegerNode) Evaluate() (int, error) {
	return node.Value, nil
}

type BinaryExpressionNode struct {
	Op          Token
	Left, Right Node
}

func (node BinaryExpressionNode) String() string {
	return fmt.Sprintf("%s %s %s", node.Left.String(), node.Op.Value, node.Right.String())
}

type VariableNode struct {
	Identifier Token
}

func (node VariableNode) String() string {
	return fmt.Sprintf("%s", node.Identifier.Value)
}

type FactorialNode struct {
	Expression Node
}

func (node FactorialNode) String() string {
	return fmt.Sprintf("%s!", node.Expression)
}

type ParenNode struct {
	Start, End Token
	Node       Node
}

func (node ParenNode) String() string {
	return fmt.Sprintf("(%s)", node.Node)
}
