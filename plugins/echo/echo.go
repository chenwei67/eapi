package echo

import (
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"regexp"
	"strings"

	"github.com/chenwei67/eapi"
	"github.com/chenwei67/eapi/plugins/common"
	"github.com/chenwei67/eapi/utils"
	"github.com/knadh/koanf"
)

var (
	routeMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"}
)

const (
	echoInstanceTypeName = "*github.com/labstack/echo/v4.Echo"
	echoGroupTypeName    = "*github.com/labstack/echo/v4.Group"
	echoGroupMethodName  = "Group"
)

type Plugin struct {
	config common.Config
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string {
	return "echo"
}

func (p *Plugin) Mount(k *koanf.Koanf) error {
	return k.Unmarshal("properties", &p.config)
}

func (p *Plugin) Analyze(ctx *eapi.Context, node ast.Node) {
	switch node := node.(type) {
	case *ast.AssignStmt:
		p.assignStmt(ctx, node)

	case *ast.CallExpr:
		p.callExpr(ctx, node)
	}
}

func (p *Plugin) assignStmt(ctx *eapi.Context, assign *ast.AssignStmt) {
	if len(assign.Rhs) != 1 || len(assign.Lhs) != 1 {
		return
	}

	callRule := eapi.NewCallRule().
		WithRule(echoInstanceTypeName, echoGroupMethodName).
		WithRule(echoGroupTypeName, echoGroupMethodName)
	for _, router := range p.config.RouterNames {
		callRule = callRule.WithRule(router, echoGroupMethodName)
	}

	rh := assign.Rhs[0]
	ctx.MatchCall(
		rh,
		callRule,
		func(callExpr *ast.CallExpr, typeName, fnName string) {
			if len(callExpr.Args) <= 0 {
				return
			}
			arg0, ok := callExpr.Args[0].(*ast.BasicLit)
			if !ok {
				return
			}
			selExpr := callExpr.Fun.(*ast.SelectorExpr)
			xIdent, ok := selExpr.X.(*ast.Ident)
			if !ok {
				return
			}
			var prefix = ""
			v := ctx.Env.Lookup(xIdent.Name)
			if rg, ok := v.(*eapi.RouteGroup); ok {
				prefix = rg.Prefix
			}
			rg := &eapi.RouteGroup{Prefix: path.Join(prefix, p.normalizePath(strings.Trim(arg0.Value, "\"")))}
			lh := assign.Lhs[0]
			lhIdent, ok := lh.(*ast.Ident)
			if !ok {
				return
			}
			switch assign.Tok {
			case token.ASSIGN:
				env := ctx.Env.Resolve(lhIdent.Name)
				if env == nil {
					ctx.Env.Define(lhIdent.Name, rg)
				} else {
					env.Assign(lhIdent.Name, rg)
				}

			case token.DEFINE:
				ctx.Env.Define(lhIdent.Name, rg)
			}
		},
	)

}

func (p *Plugin) callExpr(ctx *eapi.Context, callExpr *ast.CallExpr) {
	callRule := eapi.NewCallRule().WithRule(echoInstanceTypeName, routeMethods...).
		WithRule(echoGroupTypeName, routeMethods...)
	for _, router := range p.config.RouterNames {
		callRule = callRule.WithRule(router, routeMethods...)
	}

	ctx.MatchCall(
		callExpr,
		callRule,
		func(call *ast.CallExpr, typeName, fnName string) {
			comment := ParseCommentWithContext(ctx.GetHeadingCommentOf(call.Pos()), ctx.Package().Fset, ctx)
			if comment.Ignore() {
				return
			}
			api := p.parseAPI(ctx, callExpr, comment)
			if api == nil {
				return
			}
			ctx.AddAPI(api)
		},
	)
}

func (p *Plugin) parseAPI(ctx *eapi.Context, callExpr *ast.CallExpr, comment *eapi.Comment) (api *eapi.API) {
	if len(callExpr.Args) < 2 {
		return
	}
	arg0, ok := callExpr.Args[0].(*ast.BasicLit)
	if !ok {
		return
	}

	selExpr := callExpr.Fun.(*ast.SelectorExpr)
	var prefix string
	if xIdent, ok := selExpr.X.(*ast.Ident); ok {
		v := ctx.Env.Lookup(xIdent.Name)
		if rg, ok := v.(*eapi.RouteGroup); ok {
			prefix = rg.Prefix
		}
	}

	handlerFn := p.getHandlerFn(ctx, callExpr)
	if handlerFn == nil {
		return
	}
	typeName, methodName := utils.GetFuncInfo(handlerFn)
	handlerDef := ctx.GetDefinition(typeName, methodName)
	if handlerDef == nil {
		ctx.StrictError("handler function %s.%s not found", typeName, methodName)
		return
	}
	handlerFnDef, ok := handlerDef.(*eapi.FuncDefinition)
	if !ok {
		return
	}

	fullPath := path.Join(prefix, p.normalizePath(strings.Trim(arg0.Value, "\"")))
	method := selExpr.Sel.Name
	api = eapi.NewAPI(method, fullPath)
	api.Spec.LoadFromComment(ctx, comment)
	api.Spec.LoadFromFuncDecl(ctx, handlerFnDef.Decl)
	if api.Spec.OperationID == "" {
		id := comment.ID()
		if id == "" {
			id = handlerFnDef.Pkg().Name + "." + handlerFnDef.Decl.Name.Name
		}
		api.Spec.OperationID = id
	}
	newHandlerAnalyzer(
		ctx.NewEnv().WithPackage(handlerFnDef.Pkg()).WithFile(handlerFnDef.File()),
		api,
		handlerFnDef.Decl,
	).WithConfig(&p.config).Parse()

	return
}

func (p *Plugin) getHandlerFn(ctx *eapi.Context, callExpr *ast.CallExpr) (handlerFn *types.Func) {
	handlerArg := callExpr.Args[len(callExpr.Args)-1]
	if call, ok := handlerArg.(*ast.CallExpr); ok {
		nestedCall := utils.UnwrapCall(call)
		if len(nestedCall.Args) <= 0 {
			return
		}
		handlerArg = nestedCall.Args[0]
	}
	return ctx.GetFuncFromAstNode(handlerArg)
}

var (
	pathParamPattern = regexp.MustCompile(`:([^\/]+)`)
)

func (p *Plugin) normalizePath(path string) string {
	return pathParamPattern.ReplaceAllStringFunc(path, func(s string) string {
		s = strings.TrimPrefix(s, ":")
		return "{" + s + "}"
	})
}
