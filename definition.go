package eapi

import (
	"go/ast"
	"strings"

	"github.com/chenwei67/eapi/spec"
	"golang.org/x/tools/go/packages"
)

type Definition interface {
	Pkg() *packages.Package
	File() *ast.File
	Key() string

	definition()
}

var _ Definition = &FuncDefinition{}

type FuncDefinition struct {
	pkg  *packages.Package
	file *ast.File

	Decl *ast.FuncDecl
}

func NewFuncDefinition(pkg *packages.Package, file *ast.File, decl *ast.FuncDecl) *FuncDefinition {
	return &FuncDefinition{pkg: pkg, file: file, Decl: decl}
}

func (f *FuncDefinition) Key() string {
	if f.Decl.Recv.NumFields() == 1 {
		receiver := f.Decl.Recv.List[0]
		switch t := receiver.Type.(type) {
		case *ast.Ident:
			return f.pkg.PkgPath + "." + t.Name + "." + f.Decl.Name.Name
		case *ast.StarExpr:
			switch i := t.X.(type) {
			case *ast.Ident:
				return "*" + f.pkg.PkgPath + "." + i.Name + "." + f.Decl.Name.Name
			case *ast.IndexExpr:
				// 范型未被完全支持，暂时先这样处理
				_i, ok := i.X.(*ast.Ident)
				if !ok {
					return ""
				}
				return "*" + f.pkg.PkgPath + "." + _i.Name + "." + f.Decl.Name.Name
			default:
				LogWarn("invalid function receiver at %s", f.pkg.Fset.Position(receiver.Pos()).String())
			}
		case *ast.IndexExpr:
			// 范型未被完全支持，暂时先这样处理
			return ""
		default:
			LogWarn("invalid function receiver at %s", f.pkg.Fset.Position(receiver.Pos()).String())
		}
	}

	return f.pkg.PkgPath + "." + f.Decl.Name.Name
}

func (f *FuncDefinition) Pkg() *packages.Package {
	return f.pkg
}

func (f *FuncDefinition) File() *ast.File {
	return f.file
}

func (f *FuncDefinition) definition() {}

var _ Definition = &TypeDefinition{}

type TypeDefinition struct {
	Spec *ast.TypeSpec

	// Enum items
	Enums []*spec.ExtendedEnumItem

	pkg  *packages.Package
	file *ast.File
}

func NewTypeDefinition(pkg *packages.Package, file *ast.File, spec *ast.TypeSpec) *TypeDefinition {
	return &TypeDefinition{pkg: pkg, file: file, Spec: spec}
}

func (t *TypeDefinition) definition() {}

func (t *TypeDefinition) Pkg() *packages.Package {
	return t.pkg
}

func (t *TypeDefinition) File() *ast.File {
	return t.file
}

func (t *TypeDefinition) Key() string {
	return t.pkg.PkgPath + "." + t.Spec.Name.Name
}

func (t *TypeDefinition) ModelKey(typeArgs ...*spec.SchemaRef) string {
	sb := strings.Builder{}
	sb.WriteString(strings.ReplaceAll(t.pkg.PkgPath, "/", "_"))
	sb.WriteString(".")
	sb.WriteString(t.Spec.Name.Name)
	if len(typeArgs) > 0 {
		sb.WriteString("[")
		sb.WriteString(typeArgs[0].GetKey())
		for _, arg := range typeArgs[1:] {
			sb.WriteString(",")
			sb.WriteString(arg.GetKey())
		}
		sb.WriteString("]")
	}
	return sb.String()
}

func (t *TypeDefinition) RefKey(typeArgs ...*spec.SchemaRef) string {
	return "#/components/schemas/" + t.ModelKey(typeArgs...)
}

type Definitions map[string]Definition

func (d *Definitions) Set(def Definition) {
	k := def.Key()
	if k != "" {
		(*d)[k] = def
	}
}

func (d *Definitions) Get(key string) Definition {
	return (*d)[key]
}
