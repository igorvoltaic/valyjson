package codegen

import (
	"github.com/iv-menshenin/valyjson/generator/codegen/helpers"
	"go/ast"

	asthlp "github.com/iv-menshenin/go-ast"

	"github.com/iv-menshenin/valyjson/generator/codegen/field"
	"github.com/iv-menshenin/valyjson/generator/codegen/names"
	"github.com/iv-menshenin/valyjson/generator/codegen/tags"
)

type Struct struct {
	TaggedRenderer
	spec *ast.StructType
}

func NewStruct(name string, tags []string, spec *ast.StructType) *Struct {
	return &Struct{
		TaggedRenderer: TaggedRenderer{
			name: name,
			tags: tags,
		},
		spec: spec,
	}
}

func (s *Struct) UnmarshalFunc() []ast.Decl {
	return NewUnmarshalFunc(s.name)
}

// FillerFunc generates function code that will fill in all fields of the structure with the fastjson.Value attribute
//
//	func (s *Struct) FillFromJson(v *fastjson.Value, objPath string) (err error) { ... }
func (s *Struct) FillerFunc() ast.Decl {
	fn := asthlp.DeclareFunction(asthlp.NewIdent(names.MethodNameFill))
	fn.Comments("// " + names.MethodNameFill + " recursively fills the fields with fastjson.Value")
	fn.Receiver(asthlp.Field(names.VarNameReceiver, nil, asthlp.Star(asthlp.NewIdent(s.name))))
	fn.Params(
		asthlp.Field(names.VarNameJsonValue, nil, asthlp.Star(names.FastJsonValue)),
		asthlp.Field(names.VarNameObjPath, nil, asthlp.String),
	)
	fn.Results(
		asthlp.Field(names.VarNameError, nil, asthlp.ErrorType),
	)
	if s.tags.StrictRules() {
		fn.AppendStmt(asthlp.CommentStmt("strict rules"))
	}
	fn.AppendStmt(
		// 	if err = s.validate(v, ""); err != nil {
		//		return err
		//	}
		asthlp.MakeCallReturnIfError(nil, asthlp.Call(
			asthlp.InlineFunc(asthlp.SimpleSelector(names.VarNameReceiver, names.MethodNameValidate)),
			asthlp.NewIdent(names.VarNameJsonValue),
			asthlp.NewIdent(names.VarNameObjPath),
		)),
	)
	for _, f := range s.spec.Fields.List {
		fn.AppendStmt(fillFieldStmts(f)...)
	}
	fn.AppendStmt(asthlp.Return(asthlp.Nil))
	return fn.Decl()
}

func fillFieldStmts(fld *ast.Field) []ast.Stmt {
	var result []ast.Stmt
	factory := field.New(fld)
	for _, name := range fld.Names {
		result = append(result, factory.FillStatements(name.Name)...)
	}
	if len(fld.Names) == 0 {
		// composited struct
		var tag tags.Tags
		if fld.Tag != nil {
			tag = tags.Parse(fld.Tag.Value)
		}
		if tag.JsonAppendix() == "inline" {
			// panic("dfs")
		}
		if i, ok := fld.Type.(*ast.Ident); ok {
			result = append(result, factory.FillStatements(i.Name)...)
		}
	}
	return result
}

// ValidatorFunc generates a function declaration for validating a JSON object, taking into account the presence of fields.
//
//	func validate(v *fastjson.Value, objPath string) error {
func (s *Struct) ValidatorFunc() ast.Decl {
	fastJsonValue := asthlp.Star(names.FastJsonValue)
	fn := asthlp.DeclareFunction(asthlp.NewIdent(names.MethodNameValidate))
	fn.Comments("// " + names.MethodNameValidate + " checks for correct data structure")
	fn.Receiver(asthlp.Field(names.VarNameReceiver, nil, asthlp.Star(asthlp.NewIdent(s.name))))
	fn.Params(
		asthlp.Field(names.VarNameJsonValue, nil, fastJsonValue),
		asthlp.Field(names.VarNameObjPath, nil, asthlp.String),
	)
	fn.Results(
		asthlp.Field("", nil, asthlp.ErrorType),
	)

	const (
		keyVarName         = "key"
		jsonObjectVarName  = "o"
		checkFieldsVarName = "checkFields"
	)
	visitFunc := asthlp.DeclareFunction(nil).Params(
		asthlp.Field(keyVarName, nil, asthlp.ArrayType(asthlp.Byte)),
		asthlp.Field(asthlp.Blank.Name, nil, asthlp.Star(names.FastJsonValue)),
	)
	visitFunc.AppendStmt(
		//		if err != nil {
		//			return
		//		}
		asthlp.If(
			asthlp.NotNil(asthlp.NewIdent(names.VarNameError)),
			asthlp.ReturnEmpty(),
		),
	)
	for i, fieldSpec := range s.spec.Fields.List {
		var fieldTags tags.Tags
		if fieldSpec.Tag != nil {
			fieldTags = tags.Parse(fieldSpec.Tag.Value)
		}
		visitFunc.AppendStmt(
			//		if bytes.Equal(key, []byte{'f', 'i', 'l', 't', 'e', 'r'}) {
			//			. . .
			//		}
			asthlp.If(
				asthlp.Call(asthlp.BytesEqualFn, asthlp.NewIdent(keyVarName), asthlp.SliceByteLiteral(fieldTags.JsonName()).Expr()),
				asthlp.Increment(asthlp.Index(ast.NewIdent(checkFieldsVarName), asthlp.IntegerConstant(i))),
				asthlp.If(
					asthlp.Great(
						asthlp.Index(ast.NewIdent(checkFieldsVarName), asthlp.IntegerConstant(i)),
						asthlp.IntegerConstant(1).Expr(),
					),
					// err = fmt.Errorf("the '%s.%s' field appears in the object twice [%s]", string(key), objPath)
					asthlp.Assign(asthlp.MakeVarNames(names.VarNameError), asthlp.Assignment, asthlp.Call(
						asthlp.FmtErrorfFn,
						asthlp.StringConstant("the '%s.%s' field appears in the object twice").Expr(),
						ast.NewIdent(names.VarNameObjPath),
						asthlp.ExpressionTypeConvert(asthlp.NewIdent(keyVarName), asthlp.String),
					)),
				),
				asthlp.ReturnEmpty(),
			),
		)
	}
	//	err = fmt.Errorf("unexpected field '%s.%s'", objPath, string(key))
	if s.tags.StrictRules() {
		// If there were unregistered data fields in the JSON object, execution will surely get to that point.
		// With strict rules it is necessary to register an error
		visitFunc.AppendStmt(
			asthlp.Assign(asthlp.MakeVarNames(names.VarNameError), asthlp.Assignment, asthlp.Call(
				asthlp.FmtErrorfFn,
				asthlp.StringConstant("unexpected field '%s.%s'").Expr(),
				asthlp.NewIdent(names.VarNameObjPath),
				asthlp.ExpressionTypeConvert(asthlp.NewIdent(keyVarName), asthlp.String),
			)),
		)
	}
	fn.AppendStmt(
		//	o, err := v.Object()
		//	if err != nil {
		//		return err
		//	}
		asthlp.Assign(
			asthlp.MakeVarNames(jsonObjectVarName, names.VarNameError),
			asthlp.Definition,
			asthlp.Call(asthlp.InlineFunc(asthlp.SimpleSelector(names.VarNameJsonValue, "Object"))),
		),
		asthlp.If(asthlp.NotEqual(asthlp.NewIdent(names.VarNameError), asthlp.Nil), asthlp.Return(asthlp.NewIdent(names.VarNameError))),
	)

	if len(s.spec.Fields.List) > 0 {
		// var checkFields [1]int
		fn.AppendStmt(
			asthlp.Var(asthlp.VariableType(checkFieldsVarName, asthlp.ArrayType(asthlp.Int, asthlp.IntegerConstant(len(s.spec.Fields.List)).Expr()))),
		)
	}

	fn.AppendStmt(
		//	o.Visit(func(key []byte, _ *fastjson.Value) {
		asthlp.CallStmt(asthlp.Call(
			asthlp.InlineFunc(asthlp.SimpleSelector(jsonObjectVarName, "Visit")),
			visitFunc.Lit(),
		)),
		// return err
		asthlp.Return(asthlp.NewIdent(names.VarNameError)),
	)
	return fn.Decl()
}

// MarshalFunc marshal
//
//	func (s *S) MarshalJSON() ([]byte, error) {
//		var result = commonBuffer.Get()
//		err := s.MarshalTo(result)
//		return result.Bytes(), err
//	}
func (s *Struct) MarshalFunc() []ast.Decl {
	return NewMarshalFunc(s.name)
}

// AppendJsonFunc produces MarshalAppend(dst []byte) ([]byte, error)
func (s *Struct) AppendJsonFunc() ast.Decl {
	var fn = asthlp.DeclareFunction(asthlp.NewIdent(names.MethodNameMarshalTo)).
		Comments("// " + names.MethodNameMarshalTo + " serializes all fields of the structure using a buffer.").
		Receiver(asthlp.Field(names.VarNameReceiver, nil, asthlp.Star(ast.NewIdent(s.name)))).
		Params(asthlp.Field(names.VarNameWriter, nil, asthlp.NewIdent("Writer"))).
		Results(asthlp.Field("", nil, asthlp.ErrorType))

	if len(s.spec.Fields.List) == 0 {
		return fn.AppendStmt(
			asthlp.CallStmt(asthlp.Call(field.WriteStringFn, asthlp.StringConstant("{}").Expr())),
			asthlp.Return(asthlp.Nil),
		).Decl()
	}

	fn.AppendStmt(
		// 	if s == nil {
		//		result.WriteString("null")
		//		return nil
		//	}
		asthlp.If(
			asthlp.IsNil(asthlp.NewIdent(names.VarNameReceiver)),
			// result.WriteString("null")
			asthlp.CallStmt(asthlp.Call(field.WriteStringFn, asthlp.StringConstant("null").Expr())),
			asthlp.Return(asthlp.Nil),
		),
		// var (
		// 	err    error
		// )
		field.NeedVars(),
		// result.WriteString("{")
		asthlp.CallStmt(asthlp.Call(
			field.WriteStringFn,
			asthlp.StringConstant("{").Expr(),
		)),
	)

	for _, fld := range s.spec.Fields.List {
		fn.AppendStmt(jsonFieldStmts(fld)...)
	}

	fn.AppendStmt(
		makeWriteAndReturn("}")...,
	)
	return fn.Decl()
}

func jsonFieldStmts(fld *ast.Field) []ast.Stmt {
	if len(fld.Names) == 0 {
		factory := field.New(fld)
		name := ""
		switch t := fld.Type.(type) {

		case *ast.Ident:
			name = t.Name

		case *ast.SelectorExpr:
			name = t.Sel.Name
		}
		return factory.MarshalStatements(name)
	}
	var result []ast.Stmt
	factory := field.New(fld)
	for _, name := range fld.Names {
		result = append(result, factory.MarshalStatements(name.Name)...)
	}
	return result
}

func (s *Struct) ZeroFunc() ast.Decl {
	var fn = asthlp.DeclareFunction(asthlp.NewIdent(names.MethodNameZero)).
		Comments("// " + names.MethodNameZero + " shows whether the object is an empty value.").
		Receiver(asthlp.Field(names.VarNameReceiver, nil, ast.NewIdent(s.name))).
		Results(asthlp.Field("", nil, asthlp.Bool))

	var isArrayContains bool
	for _, fld := range s.spec.Fields.List {
		if a, ok := fld.Type.(*ast.ArrayType); ok {
			if isArrayContains = isArrayContains || a.Len != nil; isArrayContains {
				break
			}
		}
	}
	if isArrayContains {
		fn.AppendStmt(asthlp.Return(asthlp.False))
		return fn.Decl()
	}

	for _, fld := range s.spec.Fields.List {
		zero := helpers.ZeroValueOfT(fld.Type)
		for _, name := range fld.Names {
			var isNotZero = asthlp.Not(asthlp.Call(asthlp.InlineFunc(asthlp.Selector(asthlp.SimpleSelector(names.VarNameReceiver, name.Name), names.MethodNameZero))))
			if zero != nil {
				isNotZero = asthlp.NotEqual(asthlp.SimpleSelector(names.VarNameReceiver, name.Name), zero)
			}
			fn.AppendStmt(asthlp.If(isNotZero, asthlp.Return(asthlp.False)))
		}
		if len(fld.Names) == 0 {
			switch t := fld.Type.(type) {

			case *ast.Ident:
				var isNotZero = asthlp.Not(asthlp.Call(asthlp.InlineFunc(asthlp.Selector(asthlp.SimpleSelector(names.VarNameReceiver, t.Name), names.MethodNameZero))))
				if zero != nil {
					isNotZero = asthlp.NotEqual(asthlp.SimpleSelector(names.VarNameReceiver, t.Name), zero)
				}
				fn.AppendStmt(asthlp.If(isNotZero, asthlp.Return(asthlp.False)))

			case *ast.SelectorExpr:
				var isNotZero = asthlp.Not(asthlp.Call(asthlp.InlineFunc(asthlp.Selector(asthlp.SimpleSelector(names.VarNameReceiver, t.Sel.Name), names.MethodNameZero))))
				if zero != nil {
					isNotZero = asthlp.NotEqual(asthlp.SimpleSelector(names.VarNameReceiver, t.Sel.Name), helpers.ZeroValueOfT(fld.Type))
				}
				fn.AppendStmt(asthlp.If(isNotZero, asthlp.Return(asthlp.False)))
			}
		}
	}
	fn.AppendStmt(asthlp.Return(asthlp.True))
	return fn.Decl()
}
