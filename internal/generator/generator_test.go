package generator

import (
	"fmt"
	"testing"

	"github.com/chenwei67/eapi/spec"
	"github.com/stretchr/testify/assert"
)

func TestGenerator_Run(t *testing.T) {
	type args struct {
		jsCode string
		doc    *spec.T
	}
	tests := []struct {
		name       string
		args       args
		wantResult string
		wantErr    assert.ErrorAssertionFunc
	}{
		{
			name: "hello,world",
			args: args{
				jsCode: `
function print() {
	return [{code: "hello,world"}]
}
module.exports = { print }
`,
				doc: &spec.T{},
			},
			wantResult: "hello,world",
			wantErr:    assert.NoError,
		},
		{
			name: "print-basic-doc",
			args: args{
				jsCode: `
const { docBuilders: {join, indent, hardline}, printDocToString } = require("eapi");
function print(doc) {
	return [
		{
			fileName: 'code.js',
			code: printDocToString([
		"// openapi version: ", doc.openapi, hardline,
		"function hello() {", 
		indent([
			hardline,
			'return "hello,world"',
		]), hardline,
		"}",
	], { tabWidth: 2 }).formatted
		}
	];
}
module.exports = { print }
`,
				doc: &spec.T{
					OpenAPI: "3.1.0",
				},
			},
			wantResult: `// openapi version: 3.1.0
function hello() {
  return "hello,world"
}`,
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(func(key string) interface{} {
				t.Logf("get config %s", key)
				return ""
			})
			gotResult, err := g.Run(tt.args.jsCode, tt.args.doc)
			if !tt.wantErr(t, err, fmt.Sprintf("Run(%v, %v)", tt.args.jsCode, tt.args.doc)) {
				return
			}
			assert.Equalf(t, tt.wantResult, gotResult[0].Code, "Run(%v, %v)", tt.args.jsCode, tt.args.doc)
		})
	}
}
