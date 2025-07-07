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
	fmt.Printf("Process: 开始处理包路径 %s\n", packagePath)

	if len(a.plugins) <= 0 {
		panic("must register plugin before processing")
	}

	packagePath, err := filepath.Abs(packagePath)
	if err != nil {
		panic("invalid package path: " + err.Error())
	}
	fmt.Printf("Process: 绝对路径解析完成: %s\n", packagePath)

	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Process: 发生panic: %v\n", r)
			panic(r) // 重新抛出panic
		}
	}()

	var visited = make(map[string]struct{})
	fmt.Printf("Process: 开始加载包\n")
	pkgList := a.load(packagePath)
	fmt.Printf("Process: 包加载完成，共%d个包组\n", len(pkgList))

	for pkgGroupIdx, pkg := range pkgList {
		fmt.Printf("Process: 处理第%d个包组，包含%d个包\n", pkgGroupIdx+1, len(pkg))
		a.definitions = make(Definitions)

		fmt.Printf("Process: 开始加载定义\n")
		for pkgIdx, p := range pkg {
			fmt.Printf("Process: 加载第%d个包的定义: %s\n", pkgIdx+1, p.PkgPath)
			a.loadDefinitionsFromPkg(p, p.Module.Dir)
			fmt.Printf("Process: 完成第%d个包的定义加载: %s\n", pkgIdx+1, p.PkgPath)
		}
		fmt.Printf("Process: 定义加载完成\n")

		fmt.Printf("Process: 开始处理文件\n")
		for pkgIdx, pkg := range pkg {
			fmt.Printf("Process: 处理第%d个包: %s\n", pkgIdx+1, pkg.PkgPath)
			moduleDir := pkg.Module.Dir
			InspectPackage(pkg, func(pkg *packages.Package) bool {
				fmt.Printf("Process: 检查包: %s\n", pkg.PkgPath)
				if _, ok := visited[pkg.PkgPath]; ok {
					fmt.Printf("Process: 包已访问，跳过: %s\n", pkg.PkgPath)
					return false
				}
				visited[pkg.PkgPath] = struct{}{}
				if pkg.Module == nil || pkg.Module.Dir != moduleDir {
					fmt.Printf("Process: 包模块不匹配，跳过: %s\n", pkg.PkgPath)
					return false
				}
				if DEBUG {
					fmt.Printf("inspect %s\n", pkg.PkgPath)
				}

				fmt.Printf("Process: 创建上下文并处理文件，包: %s，文件数: %d\n", pkg.PkgPath, len(pkg.Syntax))
				ctx := a.context().Block().WithPackage(pkg)
				for fileIdx, file := range pkg.Syntax {
					fmt.Printf("Process: 处理第%d个文件\n", fileIdx+1)
					a.processFile(ctx.Block().WithFile(file), file, pkg)
					fmt.Printf("Process: 完成第%d个文件处理\n", fileIdx+1)
				}
				fmt.Printf("Process: 完成包处理: %s\n", pkg.PkgPath)

				return true
			})
			fmt.Printf("Process: 完成第%d个包的所有处理: %s\n", pkgIdx+1, pkg.PkgPath)
		}
		fmt.Printf("Process: 完成第%d个包组的文件处理\n", pkgGroupIdx+1)
	}

	fmt.Printf("Process: 所有处理完成\n")
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
	fmt.Printf("load: 开始加载包路径 %s\n", pkgPath)

	absPath, err := filepath.Abs(pkgPath)
	if err != nil {
		panic("invalid package path: " + pkgPath)
	}
	fmt.Printf("load: 绝对路径: %s\n", absPath)

	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("load: 发生panic: %v\n", r)
			panic(r) // 重新抛出panic
		}
	}()

	fmt.Printf("load: 创建packages.Config\n")
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
	fmt.Printf("load: packages.Config创建完成\n")

	// 使用 ./... 模式加载当前目录及所有子目录的包
	fmt.Printf("load: 使用./...模式调用packages.Load\n")
	packs, err := packages.Load(config, "./...")
	if err != nil {
		fmt.Printf("load: packages.Load失败: %v\n", err)
		panic("load packages failed: " + err.Error())
	}
	fmt.Printf("load: packages.Load成功，返回%d个包\n", len(packs))

	// 打印每个包的详细信息用于调试
	for i, p := range packs {
		fmt.Printf("load: 包%d - ID: %s, PkgPath: %s, Name: %s\n", i+1, p.ID, p.PkgPath, p.Name)
		if p.Module != nil {
			fmt.Printf("load: 包%d - Module: %s (Dir: %s)\n", i+1, p.Module.Path, p.Module.Dir)
		} else {
			fmt.Printf("load: 包%d - 无Module信息\n", i+1)
		}
		fmt.Printf("load: 包%d - 语法文件数: %d\n", i+1, len(p.Syntax))
		if len(p.Errors) > 0 {
			fmt.Printf("load: 包%d - 错误: %v\n", i+1, p.Errors)
		}
	}

	// 对于主包（command-line-arguments），手动设置Module信息
	for _, p := range packs {
		if p.PkgPath == entryPackageName && p.Module == nil {
			fmt.Printf("load: 为主包设置Module信息\n")
			module := a.parseGoModule(pkgPath)
			if module == nil {
				fmt.Printf("load: 解析go.mod失败\n")
				panic("failed to parse go.mod file in " + pkgPath)
			}
			fmt.Printf("load: 解析go.mod成功，模块路径: %s\n", module.Path)
			p.Module = module
			p.ID = module.Path
			fmt.Printf("load: 主包Module信息设置完成\n")
		}
	}

	fmt.Printf("load: 包加载完成，返回包组\n")
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
	fmt.Printf("loadDefinitionsFromPkg: 开始加载包定义: %s\n", pkg.PkgPath)

	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("loadDefinitionsFromPkg: 发生panic: %v\n", r)
			panic(r) // 重新抛出panic
		}
	}()

	var visited = make(map[string]struct{})
	InspectPackage(pkg, func(pkg *packages.Package) bool {
		fmt.Printf("loadDefinitionsFromPkg: 检查包: %s\n", pkg.PkgPath)
		if _, ok := visited[pkg.PkgPath]; ok {
			fmt.Printf("loadDefinitionsFromPkg: 包已访问，跳过: %s\n", pkg.PkgPath)
			return false
		}
		visited[pkg.PkgPath] = struct{}{}

		if pkg.Module == nil { // Go 内置包
			fmt.Printf("loadDefinitionsFromPkg: Go内置包，检查依赖: %s\n", pkg.PkgPath)
			ignore := true
			for _, depend := range a.depends {
				if strings.HasPrefix(pkg.PkgPath, depend) {
					ignore = false
					break
				}
			}
			if ignore {
				fmt.Printf("loadDefinitionsFromPkg: 内置包不在依赖中，跳过: %s\n", pkg.PkgPath)
				return false
			}
			fmt.Printf("loadDefinitionsFromPkg: 内置包在依赖中，继续处理: %s\n", pkg.PkgPath)
		} else {
			fmt.Printf("loadDefinitionsFromPkg: 检查模块目录匹配: %s (模块目录: %s, 期望: %s)\n", pkg.PkgPath, pkg.Module.Dir, moduleDir)
			if pkg.Module.Dir != moduleDir && !lo.Contains(a.depends, pkg.Module.Path) {
				fmt.Printf("loadDefinitionsFromPkg: 模块目录不匹配且不在依赖中，跳过: %s\n", pkg.PkgPath)
				return false
			}
			fmt.Printf("loadDefinitionsFromPkg: 模块检查通过，继续处理: %s\n", pkg.PkgPath)
		}

		fmt.Printf("loadDefinitionsFromPkg: 开始处理包的语法文件，共%d个文件: %s\n", len(pkg.Syntax), pkg.PkgPath)
		for fileIdx, file := range pkg.Syntax {
			fmt.Printf("loadDefinitionsFromPkg: 处理第%d个文件\n", fileIdx+1)
			ast.Inspect(file, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.FuncDecl:
					fmt.Printf("loadDefinitionsFromPkg: 找到函数定义: %s\n", node.Name.Name)
					a.definitions.Set(NewFuncDefinition(pkg, file, node))
					return false
				case *ast.TypeSpec:
					fmt.Printf("loadDefinitionsFromPkg: 找到类型定义: %s\n", node.Name.Name)
					a.definitions.Set(NewTypeDefinition(pkg, file, node))
					return false
				case *ast.GenDecl:
					if node.Tok == token.CONST {
						fmt.Printf("loadDefinitionsFromPkg: 找到常量定义\n")
						a.loadEnumDefinition(pkg, file, node)
						return false
					}
					return true
				}
				return true
			})
			fmt.Printf("loadDefinitionsFromPkg: 完成第%d个文件处理\n", fileIdx+1)
		}
		fmt.Printf("loadDefinitionsFromPkg: 完成包处理: %s\n", pkg.PkgPath)
		return true
	})
	fmt.Printf("loadDefinitionsFromPkg: 包定义加载完成: %s\n", pkg.PkgPath)
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
