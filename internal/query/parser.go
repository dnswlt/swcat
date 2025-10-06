package query

import (
	"fmt"
	"strings"
	"unicode"
)

// --- Abstract Syntax Tree (AST) ---

// Expression is the interface that all expression nodes implement.
type Expression interface {
	String() string
}

// Term represents a simple value-based search term (e.g., "bana").
type Term struct {
	Value string
}

func (t *Term) String() string {
	return t.Value
}

// AttributeTerm represents a key-operator-value term (e.g., "tag:foo").
type AttributeTerm struct {
	Attribute string
	Operator  string
	Value     string
}

func (at *AttributeTerm) String() string {
	// Add quotes if the value contains spaces or is empty, to ensure it can be re-parsed.
	if strings.Contains(at.Value, " ") || at.Value == "" {
		return fmt.Sprintf("%s%s'%s'", at.Attribute, at.Operator, at.Value)
	}
	return fmt.Sprintf("%s%s%s", at.Attribute, at.Operator, at.Value)
}

// NotExpression represents a negation using '!' (e.g., "!tag:foo").
type NotExpression struct {
	Expression Expression
}

func (ne *NotExpression) String() string {
	return fmt.Sprintf("!%s", ne.Expression.String())
}

// BinaryExpression represents a two-sided expression with an operator (AND, OR).
type BinaryExpression struct {
	Left     Expression
	Operator string // "AND" or "OR"
	Right    Expression
}

func (be *BinaryExpression) String() string {
	return fmt.Sprintf("(%s %s %s)", be.Left.String(), be.Operator, be.Right.String())
}

// --- Lexer (Tokenizer) ---

type tokenType int

const (
	tokenIllegal tokenType = iota
	tokenEOF

	// Literals
	tokenIdentifier // "bana", "tag", "foo"
	tokenString     // "'hello r'"

	// Operators
	tokenAnd // "AND"
	tokenOr  // "OR"
	tokenNot // "!"

	// Punctuation
	tokenLParen // "("
	tokenRParen // ")"
	tokenColon  // ":"
	tokenTilde  // "~"
)

var tokenNames = map[tokenType]string{
	tokenIllegal:    "ILLEGAL",
	tokenEOF:        "EOF",
	tokenIdentifier: "IDENTIFIER",
	tokenString:     "STRING",
	tokenAnd:        "AND",
	tokenOr:         "OR",
	tokenNot:        "NOT",
	tokenLParen:     "LPAREN",
	tokenRParen:     "RPAREN",
	tokenColon:      "COLON",
	tokenTilde:      "TILDE",
}

func (t tokenType) String() string {
	return tokenNames[t]
}

type token struct {
	typ tokenType
	lit string
}

type lexer struct {
	input   []rune
	pos     int // current position
	readPos int // next position to read
	ch      rune
}

func newLexer(input string) *lexer {
	l := &lexer{input: []rune(input)}
	l.readChar()
	return l
}

func (l *lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0 // EOF
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
}

func (l *lexer) nextToken() token {
	var tok token

	l.skipWhitespace()

	switch l.ch {
	case '(':
		tok = token{typ: tokenLParen, lit: "("}
	case ')':
		tok = token{typ: tokenRParen, lit: ")"}
	case ':':
		tok = token{typ: tokenColon, lit: ":"}
	case '~':
		tok = token{typ: tokenTilde, lit: "~"}
	case '!':
		tok = token{typ: tokenNot, lit: "!"}
	case '\'', '"':
		tok.typ = tokenString
		tok.lit = l.readString(l.ch)
	case 0:
		tok = token{typ: tokenEOF, lit: ""}
	default:
		if l.isIdentifierChar(l.ch) {
			lit := l.readIdentifier()
			switch lit {
			case "AND":
				return token{typ: tokenAnd, lit: "AND"}
			case "OR":
				return token{typ: tokenOr, lit: "OR"}
			default:
				return token{typ: tokenIdentifier, lit: lit}
			}
		} else {
			tok = token{typ: tokenIllegal, lit: string(l.ch)}
		}
	}

	l.readChar()
	return tok
}

func (l *lexer) skipWhitespace() {
	for unicode.IsSpace(l.ch) {
		l.readChar()
	}
}

func (l *lexer) readIdentifier() string {
	start := l.pos
	for l.isIdentifierChar(l.ch) {
		l.readChar()
	}
	return string(l.input[start:l.pos])
}

func (l *lexer) isIdentifierChar(ch rune) bool {
	return ch != 0 && !unicode.IsSpace(ch) && !strings.ContainsRune("()!:'\"~", ch)
}

func (l *lexer) readString(quote rune) string {
	l.readChar() // consume opening quote
	start := l.pos
	for l.ch != quote && l.ch != 0 {
		l.readChar()
	}
	return string(l.input[start:l.pos])
}

// --- Parser ---

type Parser struct {
	l      *lexer
	errors []string

	curToken  token
	peekToken token
}

func NewParser(l *lexer) *Parser {
	p := &Parser{l: l}
	// Read two tokens to set both curToken and peekToken
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.nextToken()
}

// Parse is the main entry point for parsing a query string.
func Parse(input string) (Expression, error) {
	l := newLexer(input)
	p := NewParser(l)
	expr := p.parseExpression(precedenceLowest)

	// After parsing, if the next token is not EOF, it means there are
	// trailing tokens that are not part of a valid expression. This is an error.
	if p.peekToken.typ != tokenEOF {
		p.errors = append(p.errors, fmt.Sprintf("unexpected token at start of expression: %s", p.peekToken.typ))
	}

	if len(p.errors) > 0 {
		return nil, fmt.Errorf("parser errors: %s", strings.Join(p.errors, "; "))
	}
	return expr, nil
}

// Pratt parser style precedence definitions
const (
	precedenceLowest = iota
	precedenceOr
	precedenceAnd
	precedenceNot
)

var precedences = map[tokenType]int{
	tokenOr:  precedenceOr,
	tokenAnd: precedenceAnd,
	// Implicit AND needs to be handled specially
}

func (p *Parser) parseExpression(precedence int) Expression {
	// Prefix parsing
	var leftExp Expression
	switch p.curToken.typ {
	case tokenNot:
		leftExp = p.parseNotExpression()
	case tokenIdentifier:
		leftExp = p.parseIdentifierOrAttributeTerm()
	case tokenLParen:
		leftExp = p.parseGroupedExpression()
	default:
		p.errors = append(p.errors, fmt.Sprintf("unexpected token at start of expression: %s", p.curToken.typ))
		return nil
	}

	// Infix parsing
	for p.peekToken.typ != tokenEOF && precedence < p.peekPrecedence() {
		// Handle explicit binary operators (AND, OR)
		switch p.peekToken.typ {
		case tokenAnd, tokenOr:
			p.nextToken()
			leftExp = p.parseBinaryExpression(leftExp)
			continue
		}

		// Handle implicit AND
		if p.isTermStart(p.peekToken.typ) {
			// Inject an implicit AND operator
			p.curToken = token{typ: tokenAnd, lit: "AND"}
			leftExp = p.parseBinaryExpression(leftExp)
		} else {
			return leftExp
		}
	}

	return leftExp
}

func (p *Parser) peekPrecedence() int {
	// Handle implicit AND
	if p.isTermStart(p.peekToken.typ) {
		return precedenceAnd
	}
	if p, ok := precedences[p.peekToken.typ]; ok {
		return p
	}
	return precedenceLowest
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.typ]; ok {
		return p
	}
	return precedenceLowest
}

// isTermStart checks if a token can be the beginning of a new term.
func (p *Parser) isTermStart(ttype tokenType) bool {
	switch ttype {
	case tokenIdentifier, tokenNot, tokenLParen:
		return true
	default:
		return false
	}
}

func (p *Parser) parseIdentifierOrAttributeTerm() Expression {
	if p.peekToken.typ == tokenColon || p.peekToken.typ == tokenTilde {
		// It's an AttributeTerm
		attrTerm := &AttributeTerm{Attribute: p.curToken.lit}
		p.nextToken() // consume identifier, current is now operator
		attrTerm.Operator = p.curToken.lit
		p.nextToken() // consume operator, current is now value

		if p.curToken.typ != tokenIdentifier && p.curToken.typ != tokenString {
			p.errors = append(p.errors, fmt.Sprintf("expected identifier or string for attribute value, got %s", p.curToken.typ))
			return nil
		}
		attrTerm.Value = p.curToken.lit
		return attrTerm
	}
	// It's a simple Term
	return &Term{Value: p.curToken.lit}
}

func (p *Parser) parseNotExpression() Expression {
	expr := &NotExpression{}
	p.nextToken() // consume '!'
	expr.Expression = p.parseExpression(precedenceNot)
	return expr
}

func (p *Parser) parseBinaryExpression(left Expression) Expression {
	expr := &BinaryExpression{
		Left:     left,
		Operator: p.curToken.lit,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	expr.Right = p.parseExpression(precedence)
	return expr
}

func (p *Parser) parseGroupedExpression() Expression {
	p.nextToken() // consume '('
	exp := p.parseExpression(precedenceLowest)
	if p.peekToken.typ != tokenRParen {
		p.errors = append(p.errors, fmt.Sprintf("expected ')' to close group, got %s", p.peekToken.typ))
		return nil
	}
	p.nextToken() // consume ')'
	return exp
}
