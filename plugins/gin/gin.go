package gin

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path"
	"regexp"
	"strings"

	analyzer "github.com/chenwei67/eapi"
	"github.com/chenwei67/eapi/plugins/common"
	"github.com/chenwei67/eapi/utils"
	"github.com/knadh/koanf"
)

var (
	routeMethods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE"}
)

const (
	ginRouterGroupTypeName = "*github.com/gin-gonic/gin.RouterGroup"
	ginIRouterTypeName     = "github.com/gin-gonic/gin.IRouter"
	ginIRoutesTypeName     = "github.com/gin-gonic/gin.IRoutes"
	routerGroupMethodName  = "Group"
)

var _ analyzer.Plugin = &Plugin{}

type Plugin struct {
	config common.Config
}

func NewPlugin() *Plugin {
	return &Plugin{}
}

func (e *Plugin) Mount(k *koanf.Koanf) error {
	err := k.Unmarshal("properties", &e.config)
	if err != nil {
		return err
	}

	return nil
}

func (e *Plugin) Analyze(ctx *analyzer.Context, node ast.Node) {
	switch n := node.(type) {
	case *ast.AssignStmt:
		e.assignStmt(ctx, n)
	case *ast.CallExpr:
		e.callExpr(ctx, n)
	}
}

func (e *Plugin) Name() string {
	return "gin"
}

func (e *Plugin) assignStmt(ctx *analyzer.Context, node ast.Node) {
	assign := node.(*ast.AssignStmt)
	if len(assign.Rhs) != 1 || len(assign.Lhs) != 1 {
		return
	}

	callRule := analyzer.NewCallRule().
		WithRule(ginRouterGroupTypeName, routerGroupMethodName).
		WithRule(ginIRouterTypeName, routerGroupMethodName).
		WithRule(ginIRoutesTypeName, routerGroupMethodName)
	for _, router := range e.config.RouterNames {
		callRule = callRule.WithRule(router, routerGroupMethodName)
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
			if rg, ok := v.(*analyzer.RouteGroup); ok {
				prefix = rg.Prefix
			}

			rg := &analyzer.RouteGroup{Prefix: path.Join(prefix, e.normalizePath(strings.Trim(arg0.Value, "\"")))}
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

	return
}

func (e *Plugin) callExpr(ctx *analyzer.Context, callExpr *ast.CallExpr) {
	callRule := analyzer.NewCallRule().WithRule(ginRouterGroupTypeName, routeMethods...).
		WithRule(ginIRouterTypeName, routeMethods...).
		WithRule(ginIRoutesTypeName, routeMethods...)
	for _, router := range e.config.RouterNames {
		callRule = callRule.WithRule(router, routeMethods...)
	}

	ctx.MatchCall(
		callExpr,
		callRule,
		func(call *ast.CallExpr, typeName, fnName string) {
			comment := analyzer.ParseCommentWithContext(ctx.GetHeadingCommentOf(call.Pos()), ctx.Package().Fset, ctx)
			if comment.Ignore() {
				return
			}
			api := e.parseAPI(ctx, callExpr, comment)
			if api == nil {
				return
			}
			ctx.AddAPI(api)
		},
	)
}

func (e *Plugin) parseAPI(ctx *analyzer.Context, callExpr *ast.CallExpr, comment *analyzer.Comment) (api *analyzer.API) {
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
		if rg, ok := v.(*analyzer.RouteGroup); ok {
			prefix = rg.Prefix
		}
	}

	handlerFn := e.getHandlerFn(ctx, callExpr)
	if handlerFn == nil {
		return
	}
	typeName, methodName := utils.GetFuncInfo(handlerFn)
	handlerDef := ctx.GetDefinition(typeName, methodName)
	if handlerDef == nil {
		ctx.StrictError("handler function %s.%s not found", typeName, methodName)
		return
	}
	handlerFnDef, ok := handlerDef.(*analyzer.FuncDefinition)
	if !ok {
		return
	}

	fullPath := path.Join(prefix, e.normalizePath(strings.Trim(arg0.Value, "\"")))
	method := selExpr.Sel.Name
	api = analyzer.NewAPI(method, fullPath)
	api.Spec.LoadFromComment(ctx, comment)
	api.Spec.LoadFromFuncDecl(ctx, handlerFnDef.Decl)
	if api.Spec.OperationID == "" {
		id := comment.ID()
		if id == "" {
			id = handlerFnDef.Pkg().Name + "." + handlerFnDef.Decl.Name.Name
		}
		api.Spec.OperationID = id
	}
	newHandlerParser(
		ctx.NewEnv().WithPackage(handlerFnDef.Pkg()).WithFile(handlerFnDef.File()),
		api,
		handlerFnDef.Decl,
	).WithConfig(&e.config).Parse()
	return
}

func (e *Plugin) getHandlerFn(ctx *analyzer.Context, callExpr *ast.CallExpr) (handlerFn *types.Func) {
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
	pathParamPattern = regexp.MustCompile(":([^\\/]+)")
)

func (e *Plugin) normalizePath(path string) string {
	return pathParamPattern.ReplaceAllStringFunc(path, func(s string) string {
		s = strings.TrimPrefix(s, ":")
		return "{" + s + "}"
	})
}
