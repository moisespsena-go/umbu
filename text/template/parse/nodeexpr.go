package parse

// ExprNode holds a number: signed or unsigned integer, float, or complex.
// The value is parsed and stored under all the types that can represent the value.
// This simulates in a small amount of code the behavior of Go's ideal constants.
type ExprNode struct {
	NodeType
	Pos
	tr   *Tree
	Op   rune
	A, B *CommandNode
}

func (t *Tree) newExpr(pos Pos, op rune, a, b *CommandNode) *ExprNode {
	return &ExprNode{tr: t, NodeType: NodeNumber, Pos: pos, Op: op, A: a, B: b}
}

func (n *ExprNode) String() string {
	return string(n.Op)
}

func (n *ExprNode) tree() *Tree {
	return n.tr
}

func (n *ExprNode) Copy() Node {
	nn := new(ExprNode)
	*nn = *n // Easy, fast, correct.
	return nn
}
