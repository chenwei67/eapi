package eapi

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenwei67/eapi/spec"
	"github.com/knadh/koanf"
	"github.com/samber/lo"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

type Analyzer struct {
	routes      APIs
	globalEnv   *Environment
	plugins     []Plugin
	definitions Definitions
	depends     []string
	k           *koanf.Koanf
	strictMode  bool

	doc      *spec.T
	packages []*packages.Package
}

func NewAnalyzer(k *koanf.Koanf) *Analyzer {
	a := &Analyzer{
		routes:      make(APIs, 0),
		globalEnv:   NewEnvironment(nil),
		plugins:     make([]Plugin, 0),
		definitions: make(Definitions),
		k:           k,
	}

	components := spec.NewComponents()
	components.Schemas = make(spec.Schemas)
	doc := &spec.T{
		OpenAPI:    "3.0.3",
		Info:       &spec.Info{},
		Components: components,
		Paths:      make(spec.Paths),
	}
	a.doc = doc

	return a
}

func (a *Analyzer) Plugin(plugins ...Plugin) *Analyzer {
	for _, plugin := range plugins {
		err := plugin.Mount(a.k)
		if err != nil {
			panic(fmt.Sprintf("mount plugin '%s' failed. error: %s", plugin.Name(), err.Error()))
		}
	}

	a.plugins = append(a.plugins, plugins...)
	return a
}

func (a *Analyzer) Depends(pkgNames ...string) *Analyzer {
	a.depends = append(a.depends, pkgNames...)
	return a
}

func (a *Analyzer) WithStrictMode(strict bool) *Analyzer {
	a.strictMode = strict
	return a
}

func (a *Analyzer) Process(packagePath string) *Analyzer {
	LogDebug("Process: 开始处理包路径 %s", packagePath)

	if len(a.plugins) <= 0 {
		panic("must register plugin before processing")
	}

	packagePath, err := filepath.Abs(packagePath)
	if err != nil {
		panic("invalid package path: " + err.Error())
	}
	LogDebug("Process: 绝对路径解析完成: %s", packagePath)

	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			LogError("Process: 发生panic: %v", r)
			panic(r) // 重新抛出panic
		}
	}()

	var visited = make(map[string]struct{})
	LogDebug("Process: 开始加载包")
	pkgList := a.load(packagePath)
	LogInfo("Process: 包加载完成，共%d个包组", len(pkgList))

	for pkgGroupIdx, pkg := range pkgList {
		LogDebug("Process: 处理第%d个包组，包含%d个包", pkgGroupIdx+1, len(pkg))
		a.definitions = make(Definitions)

		LogDebug("Process: 开始加载定义")
		for pkgIdx, p := range pkg {
			LogDebug("Process: 加载第%d个包的定义: %s", pkgIdx+1, p.PkgPath)
			a.loadDefinitionsFromPkg(p, p.Module.Dir)
			LogDebug("Process: 完成第%d个包的定义加载: %s", pkgIdx+1, p.PkgPath)
		}
		LogDebug("Process: 定义加载完成")

		LogDebug("Process: 开始处理文件")
		for pkgIdx, pkg := range pkg {
			LogDebug("Process: 处理第%d个包: %s", pkgIdx+1, pkg.PkgPath)
			moduleDir := pkg.Module.Dir
			InspectPackage(pkg, func(pkg *packages.Package) bool {
				LogDebug("Process: 检查包: %s", pkg.PkgPath)
				if _, ok := visited[pkg.PkgPath]; ok {
					LogDebug("Process: 包已访问，跳过: %s", pkg.PkgPath)
					return false
				}
				visited[pkg.PkgPath] = struct{}{}
				if pkg.Module == nil || pkg.Module.Dir != moduleDir {
					LogDebug("Process: 包模块不匹配，跳过: %s", pkg.PkgPath)
					return false
				}
				if DEBUG {
					LogDebug("inspect %s", pkg.PkgPath)
				}

				LogDebug("Process: 创建上下文并处理文件，包: %s，文件数: %d", pkg.PkgPath, len(pkg.Syntax))
				ctx := a.context().Block().WithPackage(pkg)
				for fileIdx, file := range pkg.Syntax {
					LogDebug("Process: 处理第%d个文件", fileIdx+1)
					a.processFile(ctx.Block().WithFile(file), file, pkg)
					LogDebug("Process: 完成第%d个文件处理", fileIdx+1)
				}
				LogDebug("Process: 完成包处理: %s", pkg.PkgPath)

				return true
			})
			LogDebug("Process: 完成第%d个包的所有处理: %s", pkgIdx+1, pkg.PkgPath)
		}
		LogDebug("Process: 完成第%d个包组的文件处理", pkgGroupIdx+1)
	}

	LogInfo("Process: 所有处理完成")
	return a
}

func (a *Analyzer) APIs() *APIs {
	return &a.routes
}

func (a *Analyzer) Doc() *spec.T {
	return a.doc
}

func (a *Analyzer) analyze(ctx *Context, node ast.Node) {
	for _, plugin := range a.plugins {
		plugin.Analyze(ctx, node)
	}
}

const entryPackageName = "command-line-arguments"

func (a *Analyzer) load(pkgPath string) [][]*packages.Package {
	LogDebug("load: 开始加载包路径 %s", pkgPath)

	absPath, err := filepath.Abs(pkgPath)
	if err != nil {
		panic("invalid package path: " + pkgPath)
	}
	LogDebug("load: 绝对路径: %s", absPath)

	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			LogError("load: 发生panic: %v", r)
			panic(r) // 重新抛出panic
		}
	}()

	LogDebug("load: 创建packages.Config")
	config := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedModule |
			packages.NeedTypesInfo |
			0,
		BuildFlags: []string{},
		Tests:      false,
		Dir:        absPath,
		Env:        os.Environ(),
	}
	LogDebug("load: packages.Config创建完成")

	// 使用 ./... 模式加载当前目录及所有子目录的包
	LogDebug("load: 使用./...模式调用packages.Load")
	packs, err := packages.Load(config, "./...")
	if err != nil {
		LogError("load: packages.Load失败: %v", err)
		panic("load packages failed: " + err.Error())
	}
	LogDebug("load: packages.Load成功，返回%d个包", len(packs))

	// 打印每个包的详细信息用于调试
	for i, p := range packs {
		LogDebug("load: 包%d - ID: %s, PkgPath: %s, Name: %s", i+1, p.ID, p.PkgPath, p.Name)
		if p.Module != nil {
			LogDebug("load: 包%d - Module: %s (Dir: %s)", i+1, p.Module.Path, p.Module.Dir)
		} else {
			LogDebug("load: 包%d - 无Module信息", i+1)
		}
		LogDebug("load: 包%d - 语法文件数: %d", i+1, len(p.Syntax))
		if len(p.Errors) > 0 {
			LogError("load: 包%d - 错误: %v", i+1, p.Errors)
		}
	}

	// 对于主包（command-line-arguments），手动设置Module信息
	for _, p := range packs {
		if p.PkgPath == entryPackageName && p.Module == nil {
			LogDebug("load: 为主包设置Module信息")
			module := a.parseGoModule(pkgPath)
			if module == nil {
				LogError("load: 解析go.mod失败")
				panic("failed to parse go.mod file in " + pkgPath)
			}
			LogDebug("load: 解析go.mod成功，模块路径: %s", module.Path)
			p.Module = module
			p.ID = module.Path
			LogDebug("load: 主包Module信息设置完成")
		}
	}

	LogDebug("load: 包加载完成，返回包组")
	return [][]*packages.Package{packs}
}

func (a *Analyzer) processFile(ctx *Context, file *ast.File, pkg *packages.Package) {
	comment := ctx.ParseComment(file.Doc)
	if comment.Ignore() {
		return
	}
	ctx.commentStack.comment = comment

	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.FuncDecl:
			a.funDecl(ctx.Block(), node, file, pkg)
			return false
		case *ast.BlockStmt:
			a.blockStmt(ctx.Block(), node, file, pkg)
			return false
		}

		a.analyze(ctx, node)
		return true
	})
}

func (a *Analyzer) funDecl(ctx *Context, node *ast.FuncDecl, file *ast.File, pkg *packages.Package) {
	comment := ctx.ParseComment(node.Doc)
	if comment.Ignore() {
		return
	}
	ctx.commentStack.comment = comment

	ast.Inspect(node, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.BlockStmt:
			a.blockStmt(ctx.Block(), node, file, pkg)
			return false
		}

		a.analyze(ctx, node)
		return true
	})
}

func (a *Analyzer) loadDefinitionsFromPkg(pkg *packages.Package, moduleDir string) {
	LogDebug("loadDefinitionsFromPkg: 开始加载包定义: %s", pkg.PkgPath)

	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			LogError("loadDefinitionsFromPkg: 发生panic: %v", r)
			panic(r) // 重新抛出panic
		}
	}()

	var visited = make(map[string]struct{})
	InspectPackage(pkg, func(pkg *packages.Package) bool {
		LogDebug("loadDefinitionsFromPkg: 检查包: %s", pkg.PkgPath)
		if _, ok := visited[pkg.PkgPath]; ok {
			LogDebug("loadDefinitionsFromPkg: 包已访问，跳过: %s", pkg.PkgPath)
			return false
		}
		visited[pkg.PkgPath] = struct{}{}

		if pkg.Module == nil { // Go 内置包
			LogDebug("loadDefinitionsFromPkg: Go内置包，检查依赖: %s", pkg.PkgPath)
			ignore := true
			for _, depend := range a.depends {
				if strings.HasPrefix(pkg.PkgPath, depend) {
					ignore = false
					break
				}
			}
			if ignore {
				LogDebug("loadDefinitionsFromPkg: 内置包不在依赖中，跳过: %s", pkg.PkgPath)
				return false
			}
			LogDebug("loadDefinitionsFromPkg: 内置包在依赖中，继续处理: %s", pkg.PkgPath)
		} else {
			LogDebug("loadDefinitionsFromPkg: 检查模块目录匹配: %s (模块目录: %s, 期望: %s)", pkg.PkgPath, pkg.Module.Dir, moduleDir)
			if pkg.Module.Dir != moduleDir && !lo.Contains(a.depends, pkg.Module.Path) {
				LogDebug("loadDefinitionsFromPkg: 模块目录不匹配且不在依赖中，跳过: %s", pkg.PkgPath)
				return false
			}
			LogDebug("loadDefinitionsFromPkg: 模块检查通过，继续处理: %s", pkg.PkgPath)
		}

		LogDebug("loadDefinitionsFromPkg: 开始处理包的语法文件，共%d个文件: %s", len(pkg.Syntax), pkg.PkgPath)
		for fileIdx, file := range pkg.Syntax {
			LogDebug("loadDefinitionsFromPkg: 处理第%d个文件", fileIdx+1)
			ast.Inspect(file, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.FuncDecl:
					LogDebug("loadDefinitionsFromPkg: 找到函数定义: %s", node.Name.Name)
					a.definitions.Set(NewFuncDefinition(pkg, file, node))
					return false
				case *ast.TypeSpec:
					LogDebug("loadDefinitionsFromPkg: 找到类型定义: %s", node.Name.Name)
					a.definitions.Set(NewTypeDefinition(pkg, file, node))
					return false
				case *ast.GenDecl:
					if node.Tok == token.CONST {
						LogDebug("loadDefinitionsFromPkg: 找到常量定义")
						a.loadEnumDefinition(pkg, file, node)
						return false
					}
					return true
				}
				return true
			})
			LogDebug("loadDefinitionsFromPkg: 完成第%d个文件处理", fileIdx+1)
		}
		LogDebug("loadDefinitionsFromPkg: 完成包处理: %s", pkg.PkgPath)
		return true
	})
	LogDebug("loadDefinitionsFromPkg: 包定义加载完成: %s", pkg.PkgPath)
}

type A int

const (
	A1 A = iota + 1
	A2
	A3
)

func (a *Analyzer) loadEnumDefinition(pkg *packages.Package, file *ast.File, node *ast.GenDecl) {
	for _, item := range node.Specs {
		valueSpec, ok := item.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range valueSpec.Names {
			c := pkg.TypesInfo.ObjectOf(name).(*types.Const)
			t, ok := c.Type().(*types.Named)
			if !ok {
				continue
			}
			basicType, ok := t.Underlying().(*types.Basic)
			if !ok {
				continue
			}
			pkgPath := t.Obj().Pkg().Path()
			if pkgPath != pkg.PkgPath {
				continue
			}
			def := a.definitions.Get(t.Obj().Pkg().Path() + "." + t.Obj().Name())
			if def == nil {
				continue
			}
			typeDef := def.(*TypeDefinition)
			value := ConvertStrToBasicType(c.Val().ExactString(), basicType)
			enumItem := spec.NewExtendEnumItem(name.Name, value, strings.TrimSpace(valueSpec.Doc.Text()))
			typeDef.Enums = append(typeDef.Enums, enumItem)
		}
	}
}

func (a *Analyzer) blockStmt(ctx *Context, node *ast.BlockStmt, file *ast.File, pkg *packages.Package) {
	comment := ctx.ParseComment(a.context().WithPackage(pkg).WithFile(file).GetHeadingCommentOf(node.Lbrace))
	if comment.Ignore() {
		return
	}
	ctx.commentStack.comment = comment

	a.analyze(ctx, node)

	for _, node := range node.List {
		ast.Inspect(node, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.BlockStmt:
				a.blockStmt(ctx.Block(), node, file, pkg)
				return false
			}

			a.analyze(ctx, node)
			return true
		})
	}
}

func (a *Analyzer) parseGoModule(pkgPath string) *packages.Module {
	dir, fileName := a.lookupGoModFile(pkgPath)
	if fileName == "" {
		panic("go.mod not found in " + pkgPath)
	}

	content, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		panic(err)
	}

	mod, err := modfile.Parse("go.mod", content, nil)
	if err != nil {
		panic(fmt.Sprintf("parse go.mod failed. %s. err=%s", fileName, err.Error()))
	}

	return &packages.Module{
		Path:      mod.Module.Mod.Path,
		Main:      true,
		Dir:       dir,
		GoMod:     fileName,
		GoVersion: mod.Go.Version,
	}
}

func (a *Analyzer) lookupGoModFile(pkgPath string) (string, string) {
	for {
		fileName := filepath.Join(pkgPath, "go.mod")
		_, err := os.Stat(fileName)
		if err == nil {
			return strings.TrimSuffix(pkgPath, string(filepath.Separator)), fileName
		}
		var suffix string
		pkgPath, suffix = filepath.Split(pkgPath)
		if suffix == "" {
			break
		}
	}

	return "", ""
}

func (a *Analyzer) context() *Context {
	return newContext(a, a.globalEnv)
}

func (a *Analyzer) AddRoutes(items ...*API) {
	a.routes.add(items...)

	for _, item := range items {
		path := a.doc.Paths[item.FullPath]
		if path == nil {
			path = &spec.PathItem{}
		}
		item.applyToPathItem(path)
		a.doc.Paths[item.FullPath] = path
	}
}
