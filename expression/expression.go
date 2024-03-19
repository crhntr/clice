package expression

import (
	"fmt"
	"strconv"
)

func Parse(tokens []Token) (Node, error) {
	var (
		stack []Node
	)
	i := 0
	for {
		result, consumed, err := parse(stack, tokens, i)
		if err != nil {
			return nil, err
		}
		i += consumed
		stack = result
		if i < len(tokens) {
			continue
		}
		if len(stack) < 1 {
			return nil, fmt.Errorf("parsing failed to return an expression")
		}
		if len(stack) > 1 {
			return nil, fmt.Errorf("failed build Parse tree multiple %d nodes still on stack: %#v", len(stack)-1, stack)
		}
		return stack[0], nil
	}
}

func parse(stack []Node, tokens []Token, i int) ([]Node, int, error) {
	if i >= len(tokens) {
		return nil, i, nil
	}

	token := tokens[i]

	switch token.Type {
	case TokenNumber:
		n, err := strconv.Atoi(token.Value)
		if err != nil {
			return nil, 1, fmt.Errorf("failed to Parse number  %s at expression offset %d: %w", token.Value, token.Index, err)
		}
		return append(stack, IntegerNode{Token: token, Value: n}), 1, nil
	case TokenIdentifier:
		return append(stack, IdentifierNode{Token: token}), 1, nil
	case TokenLeftParenthesis:
		var (
			totalConsumed = 1
			parenStack    []Node
		)
		i += 1
		for {
			result, consumed, err := parse(parenStack, tokens, i)
			if err != nil {
				return nil, 0, err
			}
			totalConsumed += consumed
			i += consumed
			if i >= len(tokens) {
				return nil, 0, fmt.Errorf("parenthesis at expression offset %d is missing closing parenthesis", token.Index)
			}
			if tokens[i].Type != TokenRightParenthesis {
				parenStack = result
				continue
			}
			if len(result) == 0 {
				return nil, 0, fmt.Errorf("parentheses expression is empty")
			}
			return append(stack, ParenNode{
				Node: result[0],
			}), totalConsumed + 1, nil
		}
	case TokenAdd, TokenSubtract, TokenMultiply, TokenDivide:
		node := BinaryExpressionNode{
			Op: token,
		}

		if len(stack) == 0 {
			if token.Type != TokenSubtract {
				return stack, 0, fmt.Errorf("binary expression for operator at index %d missing left hand side", token.Index)
			}
			node.Left = IntegerNode{Value: 0}
		} else {
			node.Left = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		}

		rightExpression, consumed, err := parse(nil, tokens, i+1)
		if err != nil {
			return nil, 1 + consumed, err
		}
		if len(rightExpression) != 1 {
			return stack, 0, fmt.Errorf("weird right hand expression after operator at offet %d", token.Index)
		}
		node.Right = rightExpression[0]

		if leftBinNode, ok := node.Left.(BinaryExpressionNode); ok {
			if leftBinNode.Op.BinaryOpLess(node.Op) {
				leftLeft := leftBinNode.Left
				leftRight := leftBinNode.Right
				rightNode := node.Right

				return append(stack, BinaryExpressionNode{
					Op:   leftBinNode.Op,
					Left: leftLeft,
					Right: BinaryExpressionNode{
						Op:    token,
						Left:  leftRight,
						Right: rightNode,
					},
				}), 1 + consumed, nil
			}
		}

		return append(stack, node), 1 + consumed, nil
	case TokenRightParenthesis:
		return nil, 0, fmt.Errorf("unexpected right parenthesis at expression offest %d", token.Index)
	default:
		return nil, 0, fmt.Errorf("unexpected token at index %d %s", token.Index, token.Value)
	}
}
