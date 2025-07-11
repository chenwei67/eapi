package generator

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/chenwei67/eapi/internal/jsvm"
	"github.com/chenwei67/eapi/spec"
	"github.com/link-duan/goja"
)

type Generator struct {
	vm        *jsvm.VM
	getConfig func(key string) interface{}
}

func New(getConfig func(key string) interface{}) *Generator {
	vm := jsvm.New()
	for name, code := range globalModules {
		vm.LoadModule(name, code)
	}
	g := &Generator{getConfig: getConfig}
	g.vm = vm
	return g
}

type GenerateResultItem struct {
	FileName string `json:"fileName"`
	Code     string `json:"code"`
}

func (g *Generator) Run(jsCode string, doc *spec.T) (result []*GenerateResultItem, err error) {
	const mainModule = "__main__"
	g.vm.LoadModule(mainModule, jsCode)
	return g.RunFromModule(mainModule, doc)
}

func (g *Generator) RunFromModule(module string, doc *spec.T) (result []*GenerateResultItem, err error) {
	exports, err := g.vm.Require(module)
	if err != nil {
		return
	}
	exportsObj := exports.ToObject(g.vm.VM())
	printFn, ok := goja.AssertFunction(exportsObj.Get("print"))
	if !ok {
		return nil, fmt.Errorf("print is not a function")
	}

	getConfigCallback := g.vm.VM().ToValue(func(fc goja.FunctionCall) goja.Value {
		key := fc.Argument(0).String()
		return g.vm.VM().ToValue(g.getConfig(key))
	})
	options := g.vm.VM().NewObject()
	_ = options.Set("getConfig", getConfigCallback)
	printRes, err := printFn(exportsObj, g.vm.VM().ToValue(doc), options)
	if err != nil {
		return nil, fmt.Errorf("call print() failed. %w", err)
	}
	printResJson, err := printRes.ToObject(g.vm.VM()).MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("`print()` returns invalid value. expect to get []{fileName:string;content:string}. %w", err)
	}
	err = json.Unmarshal(printResJson, &result)
	if err != nil {
		return nil, fmt.Errorf("`print()` returns invalid value. expect to get []{fileName:string;content:string}. %w", err)
	}

	return
}

func (g *Generator) transformDoc(doc *spec.T) interface{} {
	v := transformValue(reflect.ValueOf(doc))
	return v
}
