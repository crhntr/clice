package expression

import "unicode"

type Token struct {
	Type  TokenType
	Value string
	Index int
}

func (token Token) BinaryOpLess(other Token) bool {
	return token.Type < other.Type
}

type TokenType int

const (
	TokenNumber TokenType = iota
	TokenAdd
	TokenSubtract
	TokenMultiply
	TokenDivide
	TokenExponent
	TokenExclamation
	TokenLeftParenthesis
	TokenRightParenthesis
	TokenIdentifier
)

func Tokens(input string) ([]Token, error) {
	var tokens []Token

	for i := 0; i < len(input); i++ {
		c := rune(input[i])

		if unicode.IsDigit(c) {
			start := i
			dotCount := 0
			for i < len(input) && (unicode.IsDigit(rune(input[i])) || (dotCount == 0 && input[i] == '.')) {
				if input[i] == '.' {
					dotCount++
				}
				i++
			}
			tokens = append(tokens, Token{Index: start, Type: TokenNumber, Value: input[start:i]})
			i--
		} else if c == '+' {
			tokens = append(tokens, Token{Index: i, Type: TokenAdd, Value: "+"})
		} else if c == '!' {
			tokens = append(tokens, Token{Index: i, Type: TokenExclamation, Value: "!"})
		} else if c == '-' {
			tokens = append(tokens, Token{Index: i, Type: TokenSubtract, Value: "-"})
		} else if c == '*' {
			tokens = append(tokens, Token{Index: i, Type: TokenMultiply, Value: "*"})
		} else if c == '/' {
			tokens = append(tokens, Token{Index: i, Type: TokenDivide, Value: "/"})
		} else if c == '^' {
			tokens = append(tokens, Token{Index: i, Type: TokenExponent, Value: "^"})
		} else if c == '(' {
			tokens = append(tokens, Token{Index: i, Type: TokenLeftParenthesis, Value: "("})
		} else if c == ')' {
			tokens = append(tokens, Token{Index: i, Type: TokenRightParenthesis, Value: ")"})
		} else if unicode.IsSpace(c) {
			continue
		} else if unicode.IsLetter(rune(input[i])) {
			start := i
			for i < len(input) && (rune(input[i]) == '_' || unicode.IsLetter(rune(input[i])) || unicode.IsDigit(rune(input[i]))) {
				i++
			}
			tokens = append(tokens, Token{Index: start, Type: TokenIdentifier, Value: input[start:i]})
			i--
		}
	}

	return tokens, nil
}
