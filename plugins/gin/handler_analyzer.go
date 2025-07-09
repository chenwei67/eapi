package gin

import (
	"go/ast"
	"net/http"
	"os"
	"strconv"
	"strings"

	analyzer "github.com/chenwei67/eapi"
	"github.com/chenwei67/eapi/plugins/common"
	"github.com/chenwei67/eapi/spec"
	"github.com/chenwei67/eapi/tag"
	"github.com/iancoleman/strcase"
	"github.com/samber/lo"
)

const ginContextIdentName = "*github.com/gin-gonic/gin.Context"

var (
	interestedGinContextMethods = []string{
		"Bind",
		"BindJSON",
		"BindXML",
		"BindYAML",
		"BindTOML",
		"BindUri",
		"ShouldBind",
		"MustBindWith",
		"ShouldBindJSON",
		"ShouldBindXML",
		"ShouldBindYAML",
		"ShouldBindTOML",
		"ShouldBindUri",
		"ShouldBindHeader",
		"ShouldBindWith",
		"ShouldBindQuery",
		"JSON",
		"Query",
		"Param",
		"PostForm",
		"PostFormArray",
		"GetPostForm",
		"GetPostFormArray",
		"XML",
		"Redirect",
		"FormFile",
		"DefaultQuery",
		"DefaultPostForm",
	}
)

type handlerAnalyzer struct {
	ctx  *analyzer.Context
	api  *analyzer.API
	spec *analyzer.APISpec
	decl *ast.FuncDecl

	c *common.Config
}

func newHandlerParser(ctx *analyzer.Context, api *analyzer.API, decl *ast.FuncDecl) *handlerAnalyzer {
	return &handlerAnalyzer{ctx: ctx, api: api, spec: api.Spec, decl: decl}
}

func (p *handlerAnalyzer) WithConfig(c *common.Config) *handlerAnalyzer {
	p.c = c
	return p
}

func (p *handlerAnalyzer) Parse() {
	ast.Inspect(p.decl, func(node ast.Node) bool {
		customRuleAnalyzer := common.NewCustomRuleAnalyzer(
			p.ctx,
			p.spec,
			p.api,
			p.c,
		)
		matched := customRuleAnalyzer.MatchCustomResponseRule(node)
		if matched {
			return true
		}
		matched = customRuleAnalyzer.MatchCustomRequestRule(node)
		if matched {
			return true
		}

		p.ctx.MatchCall(node,
			analyzer.NewCallRule().WithRule(ginContextIdentName, interestedGinContextMethods...),
			func(call *ast.CallExpr, typeName, fnName string) {
				switch fnName {
				case "Bind", "ShouldBind":
					p.parseBinding(call)
				case "BindJSON", "ShouldBindJSON":
					p.parseBindWithContentType(call, analyzer.MimeTypeJson)
				case "BindXML", "ShouldBindXML":
					p.parseBindWithContentType(call, analyzer.MimeApplicationXml)
				case "BindYAML", "ShouldBindYAML":
					p.parseBindWithContentType(call, "application/yaml")
				case "BindTOML", "ShouldBindTOML":
					p.parseBindWithContentType(call, "application/toml")
				case "BindUri", "ShouldBindUri":
					p.parseBindUri(call)
				case "BindHeader", "ShouldBindHeader":
					// TODO
				case "ShouldBindWith", "MustBindWith":
					p.parseBindWith(call)
				case "JSON":
					p.parseResBody(call, analyzer.MimeTypeJson)
				case "XML":
					p.parseResBody(call, analyzer.MimeApplicationXml)
				case "Query", "ShouldBindQuery": // query parameter
					p.parsePrimitiveParam(call, "query")
				case "Param": // path parameter
					p.parsePrimitiveParam(call, "path")
				case "PostForm", "GetPostForm":
					p.parseFormData(call, "string")
				case "FormFile":
					p.parseFormData(call, "file")
				case "PostFormArray", "GetPostFormArray":
					p.parseFormData(call, spec.TypeArray, func(s *spec.Schema) {
						s.Items = spec.NewStringSchema()
						s.ExtendedTypeInfo = &spec.ExtendedTypeInfo{
							Type:  spec.ExtendedTypeArray,
							Items: s.Items,
						}
					})
				case "Redirect":
					p.parseRedirectRes(call)
				case "DefaultQuery":
					p.parsePrimitiveParamWithDefault(call, "query")
				case "DefaultPostForm":
					p.parseFormData(call, "string")
					// TODO: supporting more methods (FileForm(), HTML(), Data(), etc...)
				}
			},
		)
		return true
	})
}

func (p *handlerAnalyzer) paramNameParser(fieldName string, tags map[string]string) (name, in string) {
	name, ok := tags["form"]
	if ok {
		name, _, _ = strings.Cut(name, ",")
		return name, "query"
	}
	return fieldName, "query"
}

func (p *handlerAnalyzer) parseBinding(call *ast.CallExpr) {
	if len(call.Args) != 1 {
		return
	}
	arg0 := call.Args[0]

	switch p.api.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodDelete:
		params := analyzer.NewParamParser(p.ctx, p.paramNameParser).Parse(arg0)
		for _, param := range params {
			p.spec.AddParameter(param)
		}
	default:
		contentType := p.getDefaultContentType()
		p.parseBindWithContentType(call, contentType)
	}
}

func (p *handlerAnalyzer) parseBindWithContentType(call *ast.CallExpr, contentType string) {
	if len(call.Args) != 1 {
		return
	}
	arg0 := call.Args[0]

	schema := p.ctx.GetSchemaByExpr(arg0, contentType)
	if schema == nil {
		return
	}
	commentGroup := p.ctx.GetHeadingCommentOf(call.Pos())
	if commentGroup != nil {
		comment := p.ctx.ParseComment(commentGroup)
		schema.Description = comment.Text()
	}
	reqBody := spec.NewRequestBody().WithSchemaRef(schema, []string{contentType})
	p.spec.RequestBody = reqBody
}

func (p *handlerAnalyzer) parseResBody(call *ast.CallExpr, contentType string) {
	if len(call.Args) != 2 {
		return
	}

	res := spec.NewResponse()
	commentGroup := p.ctx.GetHeadingCommentOf(call.Pos())
	if commentGroup != nil {
		comment := p.ctx.ParseComment(commentGroup)
		res.Description = comment.TextPointer()
	}

	schema := p.ctx.GetSchemaByExpr(call.Args[1], contentType)
	res.Content = spec.NewContentWithSchemaRef(schema, []string{contentType})
	statusCode := p.ctx.ParseStatusCode(call.Args[0])
	p.spec.AddResponse(statusCode, res)
}

func (p *handlerAnalyzer) parseRedirectRes(call *ast.CallExpr) {
	if len(call.Args) == 0 {
		return
	}

	res := spec.NewResponse()
	commentGroup := p.ctx.GetHeadingCommentOf(call.Pos())
	if commentGroup != nil {
		comment := p.ctx.ParseComment(commentGroup)
		if comment != nil {
			desc := comment.Text()
			res.Description = &desc
		}
	}
	statusCode := p.ctx.ParseStatusCode(call.Args[0])
	p.spec.AddResponse(statusCode, res)
}

func (p *handlerAnalyzer) parsePrimitiveParam(call *ast.CallExpr, in string) {
	param := p.primitiveParam(call, in)
	p.spec.AddParameter(param)
}

func (p *handlerAnalyzer) parsePrimitiveParamWithDefault(call *ast.CallExpr, in string) {
	param := p.primitiveParamWithDefault(call, in)
	p.spec.AddParameter(param)
}

func (p *handlerAnalyzer) parseFormData(call *ast.CallExpr, fieldType string, options ...func(s *spec.Schema)) {
	if len(call.Args) <= 0 {
		return
	}
	arg0 := call.Args[0]
	arg0Lit, ok := arg0.(*ast.BasicLit)
	if !ok {
		return
	}

	name := strings.Trim(arg0Lit.Value, "\"")
	paramSchema := spec.NewSchema()
	paramSchema.Title = name
	paramSchema.Type = fieldType
	for _, option := range options {
		option(paramSchema)
	}

	requestBody := p.spec.RequestBody
	if requestBody == nil {
		requestBody = spec.NewRequestBody().WithContent(spec.NewContent())
		p.spec.RequestBody = requestBody
	}
	mediaType := requestBody.GetMediaType(analyzer.MimeTypeFormData)
	if mediaType == nil {
		mediaType = spec.NewMediaType()
		requestBody.Content[analyzer.MimeTypeFormData] = mediaType
	}

	comment := p.ctx.ParseComment(p.ctx.GetHeadingCommentOf(call.Pos()))
	paramSchema.Description = comment.Text()

	var schemaRef = mediaType.Schema
	var schema *spec.SchemaRef
	if schemaRef != nil {
		schema = spec.Unref(p.ctx.Doc(), schemaRef)
		schema.WithProperty(name, paramSchema)
	} else {
		schema = spec.NewObjectSchema().NewRef()
		title := strcase.ToCamel(p.spec.OperationID) + "Request"
		schema.Title = title
		schema.WithProperty(name, paramSchema)
		p.ctx.Doc().Components.Schemas[title] = schema
		schemaRef = spec.RefComponentSchemas(title)
		mediaType.Schema = schemaRef
	}
	if comment.Required() {
		schema.Required = append(schema.Required, name)
	}
}

func (p *handlerAnalyzer) primitiveParamWithDefault(call *ast.CallExpr, in string) *spec.Parameter {
	if len(call.Args) < 2 {
		return nil
	}
	param := p.primitiveParam(call, in)
	if param == nil {
		return nil
	}

	arg1Lit, ok := call.Args[1].(*ast.BasicLit)
	if !ok {
		return nil
	}
	param.Schema.Default, _ = strconv.Unquote(arg1Lit.Value)

	return param
}

func (p *handlerAnalyzer) primitiveParam(call *ast.CallExpr, in string) *spec.Parameter {
	if len(call.Args) <= 0 {
		return nil
	}
	arg0 := call.Args[0]
	arg0Lit, ok := arg0.(*ast.BasicLit)
	if !ok {
		return nil
	}
	name := strings.Trim(arg0Lit.Value, "\"")
	paramSchema := spec.NewSchema()
	paramSchema.Title = name
	paramSchema.Type = "string"

	comment := p.ctx.ParseComment(p.ctx.GetHeadingCommentOf(call.Pos()))

	var res *spec.Parameter
	switch in {
	case "path":
		res = spec.NewPathParameter(name).WithSchema(paramSchema)
	case "query":
		res = spec.NewQueryParameter(name).WithSchema(paramSchema)
		res.Required = comment.Required()
	default:
		return nil
	}

	res.Description = comment.Text()
	return res
}

// 获取一个尽可能正确的 request payload contentType
func (p *handlerAnalyzer) getDefaultContentType() string {
	if len(p.spec.Consumes) != 0 {
		return p.spec.Consumes[0]
	}

	// fallback
	switch p.api.Method {
	case http.MethodGet, http.MethodHead:
		return analyzer.MimeTypeFormData
	default:
		return analyzer.MimeTypeJson
	}
}

func (p *handlerAnalyzer) parseBindUri(call *ast.CallExpr) {
	if len(call.Args) != 1 {
		return
	}
	arg0 := call.Args[0]

	analyzer.NewSchemaBuilder(p.ctx, "").WithFieldNameParser(p.parseUriFieldName).ParseExpr(arg0)
	schema := p.ctx.GetSchemaByExpr(arg0, "")
	if schema == nil {
		return
	}
	schema = schema.Unref(p.ctx.Doc())
	if schema == nil {
		return
	}
	for name, property := range schema.Properties {
		p.api.Spec.Parameters = lo.Filter(p.api.Spec.Parameters, func(ref *spec.ParameterRef, i int) bool { return ref.Name != name })
		param := spec.NewPathParameter(name).WithSchema(property)
		param.Description = property.Description
		p.api.Spec.AddParameter(param)
	}
}

func (p *handlerAnalyzer) parseUriFieldName(name string, field *ast.Field) string {
	tags := tag.Parse(field.Tag.Value)
	uriTag, ok := tags["uri"]
	if !ok {
		return name
	}
	res, _, _ := strings.Cut(uriTag, ",")
	return res
}

// parseBindWith 处理 BindWith, ShouldBindWith, MustBindWith 方法
// 这些方法的第二个参数指定了绑定类型
func (p *handlerAnalyzer) parseBindWith(call *ast.CallExpr) {
	if len(call.Args) < 2 {
		analyzer.LogWarn("BindWith 方法至少需要两个参数")
		return
	}

	// 第一个参数是要绑定的结构体
	arg0 := call.Args[0]
	// 第二个参数是绑定类型
	arg1 := call.Args[1]

	// 尝试从第二个参数推断内容类型
	contentType := p.getContentTypeFromBinding(arg1)
	if contentType == "" {
		analyzer.LogWarn("无法从绑定类型 %s 推断内容类型", arg1)
		os.Exit(1)
	}
	analyzer.LogDebug("推断出的内容类型: %s", contentType)
	// 使用推断出的内容类型进行绑定
	schema := p.ctx.GetSchemaByExpr(arg0, contentType)
	if schema == nil {
		analyzer.LogWarn("无法获取绑定的 schema，可能是因为表达式 %s 无法解析", arg0)
		os.Exit(1)
	}
	commentGroup := p.ctx.GetHeadingCommentOf(call.Pos())
	if commentGroup != nil {
		comment := p.ctx.ParseComment(commentGroup)
		schema.Description = comment.Text()
	}
	reqBody := spec.NewRequestBody().WithSchemaRef(schema, []string{contentType})
	p.spec.RequestBody = reqBody
}

// getContentTypeFromBinding 根据绑定类型推断内容类型
func (p *handlerAnalyzer) getContentTypeFromBinding(bindingExpr ast.Expr) string {
	// 处理选择器表达式，如 binding.JSON
	if sel, ok := bindingExpr.(*ast.SelectorExpr); ok {
		switch sel.Sel.Name {
		case "JSON":
			return analyzer.MimeTypeJson
		case "XML":
			return analyzer.MimeApplicationXml
		case "YAML":
			return "application/yaml"
		case "TOML":
			return "application/toml"
		case "Form":
			return analyzer.MimeTypeFormData
		case "FormPost":
			return analyzer.MimeTypeFormData
		case "FormMultipart":
			return "multipart/form-data"
		case "ProtoBuf":
			return "application/x-protobuf"
		case "MsgPack":
			return "application/x-msgpack"
		case "Query":
			return "application/x-www-form-urlencoded"
		case "Uri":
			return "" // Uri 绑定不需要内容类型
		case "Header":
			return "" // Header 绑定不需要内容类型
		}
	}

	// 处理标识符，如直接使用变量
	if ident, ok := bindingExpr.(*ast.Ident); ok {
		// 这里可以根据变量名进行推断，但通常需要更复杂的类型分析
		_ = ident
	}

	return ""
}
