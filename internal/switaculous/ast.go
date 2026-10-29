package switaculous

// ---------- Types ----------

type TypeRef struct {
	Kind   int // 0 named, 1 array, 2 dict, 3 func, 4 tuple
	Name   string
	Args   []*TypeRef
	Elem   *TypeRef
	Key    *TypeRef
	Params []*TypeRef
	Result *TypeRef
	Opt    bool
	Some   bool
}

const (
	tyNamed = iota
	tyArray
	tyDict
	tyFunc
	tyTuple
)

func (t *TypeRef) String() string {
	if t == nil {
		return "?"
	}
	s := ""
	switch t.Kind {
	case tyNamed:
		s = t.Name
		if len(t.Args) > 0 {
			s += "<"
			for i, a := range t.Args {
				if i > 0 {
					s += ", "
				}
				s += a.String()
			}
			s += ">"
		}
	case tyArray:
		s = "[" + t.Elem.String() + "]"
	case tyDict:
		s = "[" + t.Key.String() + ": " + t.Elem.String() + "]"
	case tyFunc:
		s = "("
		for i, a := range t.Params {
			if i > 0 {
				s += ", "
			}
			s += a.String()
		}
		s += ") -> " + t.Result.String()
	case tyTuple:
		s = "("
		for i, a := range t.Params {
			if i > 0 {
				s += ", "
			}
			s += a.String()
		}
		s += ")"
	}
	if t.Some {
		s = "some " + s
	}
	if t.Opt {
		s += "?"
	}
	return s
}

// ---------- Expressions ----------

type Expr interface{ P() Pos }

type IntLit struct {
	Pos Pos
	V   int64
}
type FloatLit struct {
	Pos Pos
	V   float64
}
type StrPiece struct {
	Lit  string
	X    Expr
	Args []Arg // extra interpolation args (specifier:, format:)
}
type StrLit struct {
	Pos    Pos
	Pieces []StrPiece
}
type BoolLit struct {
	Pos Pos
	V   bool
}
type NilLit struct{ Pos Pos }
type Ident struct {
	Pos  Pos
	Name string
}
type ImplicitMember struct {
	Pos  Pos
	Name string
}
type MemberExpr struct {
	Pos  Pos
	X    Expr
	Name string
}
type Arg struct {
	Label    string
	X        Expr
	Trailing bool
}
type CallExpr struct {
	Pos  Pos
	Fn   Expr
	Args []Arg
}
type SubscriptExpr struct {
	Pos  Pos
	X    Expr
	Args []Arg
}
type OptChain struct {
	Pos Pos
	X   Expr
}
type OptMark struct {
	Pos Pos
	X   Expr
}
type ForceUnwrap struct {
	Pos Pos
	X   Expr
}
type UnaryExpr struct {
	Pos Pos
	Op  string
	X   Expr
}
type PostfixRange struct {
	Pos Pos
	X   Expr
}
type BinaryExpr struct {
	Pos  Pos
	Op   string
	L, R Expr
}
type AssignExpr struct {
	Pos  Pos
	Op   string
	L, R Expr
}
type TernaryExpr struct {
	Pos     Pos
	C, T, F Expr
}
type CastExpr struct {
	Pos  Pos
	X    Expr
	Kind string
	T    *TypeRef
}
type ClosureExpr struct {
	Pos       Pos
	Params    []*Param
	Body      []Stmt
	HasSig    bool
	MaxDollar int
}
type ArrayLit struct {
	Pos   Pos
	Elems []Expr
}
type DictLit struct {
	Pos        Pos
	Keys, Vals []Expr
}
type TupleExpr struct {
	Pos    Pos
	Elems  []Expr
	Labels []string
}
type GenericTypeExpr struct {
	Pos  Pos
	X    Expr
	Args []*TypeRef
}
type KeyPathExpr struct {
	Pos  Pos
	Root string
	Path []string
}
type TryExpr struct {
	Pos  Pos
	Kind string
	X    Expr
}
type AwaitExpr struct {
	Pos Pos
	X   Expr
}
type InOutExpr struct {
	Pos Pos
	X   Expr
}
type TypeExpr struct {
	Pos Pos
	T   *TypeRef
}
type IfExpr struct {
	Pos Pos
	S   *IfStmt
}
type SwitchExpr struct {
	Pos Pos
	S   *SwitchStmt
}

func (e *IntLit) P() Pos          { return e.Pos }
func (e *FloatLit) P() Pos        { return e.Pos }
func (e *StrLit) P() Pos          { return e.Pos }
func (e *BoolLit) P() Pos         { return e.Pos }
func (e *NilLit) P() Pos          { return e.Pos }
func (e *Ident) P() Pos           { return e.Pos }
func (e *ImplicitMember) P() Pos  { return e.Pos }
func (e *MemberExpr) P() Pos      { return e.Pos }
func (e *CallExpr) P() Pos        { return e.Pos }
func (e *SubscriptExpr) P() Pos   { return e.Pos }
func (e *OptChain) P() Pos        { return e.Pos }
func (e *OptMark) P() Pos         { return e.Pos }
func (e *ForceUnwrap) P() Pos     { return e.Pos }
func (e *UnaryExpr) P() Pos       { return e.Pos }
func (e *PostfixRange) P() Pos    { return e.Pos }
func (e *BinaryExpr) P() Pos      { return e.Pos }
func (e *AssignExpr) P() Pos      { return e.Pos }
func (e *TernaryExpr) P() Pos     { return e.Pos }
func (e *CastExpr) P() Pos        { return e.Pos }
func (e *ClosureExpr) P() Pos     { return e.Pos }
func (e *ArrayLit) P() Pos        { return e.Pos }
func (e *DictLit) P() Pos         { return e.Pos }
func (e *TupleExpr) P() Pos       { return e.Pos }
func (e *KeyPathExpr) P() Pos     { return e.Pos }
func (e *GenericTypeExpr) P() Pos { return e.Pos }
func (e *TryExpr) P() Pos         { return e.Pos }
func (e *AwaitExpr) P() Pos       { return e.Pos }
func (e *InOutExpr) P() Pos       { return e.Pos }
func (e *TypeExpr) P() Pos        { return e.Pos }
func (e *IfExpr) P() Pos          { return e.Pos }
func (e *SwitchExpr) P() Pos      { return e.Pos }

// ---------- Statements & Declarations ----------

type Stmt interface{}

type Attr struct {
	Name string
	Args []Arg
	Pos  Pos
}

type Param struct {
	Label    string // external label ("" for _)
	Name     string
	Type     *TypeRef
	Default  Expr
	Attrs    []*Attr
	Variadic bool
	InOut    bool
}

type ExprStmt struct{ X Expr }

type VarDecl struct {
	Pos      Pos
	Let      bool
	Name     string
	Pat      Pattern // tuple destructuring
	Type     *TypeRef
	Init     Expr
	Attrs    []*Attr
	Mods     map[string]bool
	Get      []Stmt
	Set      []Stmt
	SetName  string
	WillSet  []Stmt
	DidSet   []Stmt
	Computed bool
}

type FuncDecl struct {
	Pos         Pos
	Name        string
	Params      []*Param
	Result      *TypeRef
	Body        []Stmt
	HasBody     bool
	Attrs       []*Attr
	Mods        map[string]bool
	Throws      bool
	Async       bool
	IsInit      bool
	Failable    bool
	IsSubscript bool
	SetBody     []Stmt
	SetName     string
	Generics    []string
}

type EnumCase struct {
	Name   string
	Params []*Param
	Raw    Expr
	Pos    Pos
}

type EnumCaseDecl struct {
	Pos   Pos
	Cases []*EnumCase
}

type TypeDecl struct {
	Pos      Pos
	Kind     string // struct class enum protocol extension
	Name     string
	Inherits []string
	Members  []Stmt
	Attrs    []*Attr
	Mods     map[string]bool
	Generics []string
	Target   *TypeRef            // the extended type, for extensions
	SameType map[string]*TypeRef // where Element == X, Self == X
}

type TypeAliasDecl struct {
	Name string
	T    *TypeRef
}

type PreviewDecl struct {
	Pos  Pos
	Name string
	Body []Stmt
}

type Cond struct {
	Kind string // expr, let, var, case
	Pat  Pattern
	Name string
	X    Expr
	Type *TypeRef
}

type IfStmt struct {
	Pos   Pos
	Conds []Cond
	Then  []Stmt
	Else  Stmt
}
type GuardStmt struct {
	Pos   Pos
	Conds []Cond
	Else  []Stmt
}
type ForStmt struct {
	Pos   Pos
	Pat   Pattern
	Seq   Expr
	Where Expr
	Body  []Stmt
	Label string
}
type WhileStmt struct {
	Pos   Pos
	Conds []Cond
	Body  []Stmt
	Label string
}
type RepeatStmt struct {
	Pos   Pos
	Body  []Stmt
	Cond  Expr
	Label string
}
type SwitchCase struct {
	Pats    []Pattern
	Where   Expr
	Body    []Stmt
	Default bool
	Pos     Pos
}
type SwitchStmt struct {
	Pos   Pos
	Subj  Expr
	Cases []*SwitchCase
	Label string
}
type ReturnStmt struct {
	Pos Pos
	X   Expr
}
type BreakStmt struct{ Label string }
type ContinueStmt struct{ Label string }
type FallthroughStmt struct{}
type ThrowStmt struct {
	Pos Pos
	X   Expr
}
type DeferStmt struct{ Body []Stmt }
type CatchClause struct {
	Pat   Pattern
	Where Expr
	Body  []Stmt
}
type DoStmt struct {
	Body    []Stmt
	Catches []*CatchClause
}
type BlockStmt struct{ Body []Stmt }

// ---------- Patterns ----------

type Pattern interface{}

type WildPat struct{}
type BindPat struct{ Name string }
type ExprPat struct{ X Expr }
type EnumPat struct {
	Type   string
	Case   string
	Subs   []Pattern
	Labels []string
	HasSub bool
}
type TuplePat struct {
	Elems  []Pattern
	Labels []string
}
type IsPat struct{ T *TypeRef }
type AsPat struct {
	Sub Pattern
	T   *TypeRef
}
type OptPat struct{ Sub Pattern }
