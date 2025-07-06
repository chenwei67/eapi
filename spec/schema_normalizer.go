package spec

import (
	"fmt"
	"strings"
)

type schemaNormalizer struct {
	doc *T
}

func newSchemaNormalizer(doc *T) *schemaNormalizer {
	return &schemaNormalizer{doc: doc}
}

func (s *schemaNormalizer) normalize() *T {
	fmt.Printf("normalize: 开始规范化处理\n")
	
	// 处理Components.Schemas
	fmt.Printf("normalize: 开始处理Components.Schemas，共%d个schema\n", len(s.doc.Components.Schemas))
	for key, ref := range s.doc.Components.Schemas {
		fmt.Printf("normalize: 处理schema key=%s\n", key)
		if ref == nil || ref.Ref != "" {
			fmt.Printf("normalize: 跳过schema key=%s (ref为空或有引用)\n", key)
			continue
		}
		ext := ref.ExtendedTypeInfo
		if ext == nil || ext.Type != ExtendedTypeSpecific {
			fmt.Printf("normalize: 跳过schema key=%s (ExtendedTypeInfo为空或类型不匹配)\n", key)
			continue
		}

		fmt.Printf("normalize: 开始处理特定类型schema key=%s\n", key)
		s.doc.Components.Schemas[key] = s.process(ext.SpecificType.Type, ext.SpecificType.Args)
		fmt.Printf("normalize: 完成处理特定类型schema key=%s\n", key)
	}
	
	// 处理Paths
	fmt.Printf("normalize: 开始处理Paths，共%d个路径\n", len(s.doc.Paths))
	for pathKey, item := range s.doc.Paths {
		fmt.Printf("normalize: 开始处理路径 %s\n", pathKey)
		s.processPathItem(item)
		fmt.Printf("normalize: 完成处理路径 %s\n", pathKey)
	}
	
	fmt.Printf("normalize: 规范化处理完成\n")
	return s.doc
}

func (s *schemaNormalizer) processPathItem(item *PathItem) {
	fmt.Printf("processPathItem: 开始处理PathItem\n")
	
	if item == nil {
		fmt.Printf("processPathItem: PathItem为nil，跳过处理\n")
		return
	}
	
	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("processPathItem: 发生panic: %v\n", r)
			panic(r) // 重新抛出panic
		}
	}()
	
	fmt.Printf("processPathItem: 处理Connect操作\n")
	s.processOperation(item.Connect)
	
	fmt.Printf("processPathItem: 处理Delete操作\n")
	s.processOperation(item.Delete)
	
	fmt.Printf("processPathItem: 处理Get操作\n")
	s.processOperation(item.Get)
	
	fmt.Printf("processPathItem: 处理Head操作\n")
	s.processOperation(item.Head)
	
	fmt.Printf("processPathItem: 处理Options操作\n")
	s.processOperation(item.Options)
	
	fmt.Printf("processPathItem: 处理Patch操作\n")
	s.processOperation(item.Patch)
	
	fmt.Printf("processPathItem: 处理Post操作\n")
	s.processOperation(item.Post)
	
	fmt.Printf("processPathItem: 处理Put操作\n")
	s.processOperation(item.Put)
	
	fmt.Printf("processPathItem: 处理Trace操作\n")
	s.processOperation(item.Trace)
	
	fmt.Printf("processPathItem: PathItem处理完成\n")
}

func (s *schemaNormalizer) processSchemaRef(ref *Schema) *Schema {
	fmt.Printf("processSchemaRef: 开始处理SchemaRef\n")
	
	if ref == nil {
		fmt.Printf("processSchemaRef: Schema为nil，返回nil\n")
		return ref
	}
	
	if ref.Ref != "" {
		fmt.Printf("processSchemaRef: Schema有引用 %s，直接返回\n", ref.Ref)
		return ref
	}
	
	ext := ref.ExtendedTypeInfo
	if ext == nil {
		fmt.Printf("processSchemaRef: ExtendedTypeInfo为nil，直接返回\n")
		return ref
	}
	
	if ext.Type != ExtendedTypeSpecific {
		fmt.Printf("processSchemaRef: ExtendedTypeInfo类型不是ExtendedTypeSpecific (实际类型: %v)，直接返回\n", ext.Type)
		return ref
	}
	
	fmt.Printf("processSchemaRef: 开始处理特定类型，调用process方法\n")
	
	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("processSchemaRef: 在process方法中发生panic: %v\n", r)
			panic(r) // 重新抛出panic
		}
	}()
	
	result := s.process(ext.SpecificType.Type, ext.SpecificType.Args)
	fmt.Printf("processSchemaRef: process方法执行完成\n")
	return result
}

func (s *schemaNormalizer) processOperation(op *Operation) {
	fmt.Printf("processOperation: 开始处理Operation\n")
	
	if op == nil {
		fmt.Printf("processOperation: Operation为nil，跳过处理\n")
		return
	}
	
	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("processOperation: 发生panic: %v\n", r)
			panic(r) // 重新抛出panic
		}
	}()
	
	fmt.Printf("processOperation: 处理Responses，共%d个响应\n", len(op.Responses))
	for responseCode, ref := range op.Responses {
		fmt.Printf("processOperation: 处理响应码 %s\n", responseCode)
		if ref == nil {
			fmt.Printf("processOperation: 响应码 %s 的ref为nil\n", responseCode)
			continue
		}
		fmt.Printf("processOperation: 处理响应码 %s 的Content，共%d个媒体类型\n", responseCode, len(ref.Content))
		for mediaTypeKey, mediaType := range ref.Content {
			fmt.Printf("processOperation: 处理媒体类型 %s\n", mediaTypeKey)
			if mediaType == nil {
				fmt.Printf("processOperation: 媒体类型 %s 为nil\n", mediaTypeKey)
				continue
			}
			fmt.Printf("processOperation: 开始处理媒体类型 %s 的Schema\n", mediaTypeKey)
			mediaType.Schema = s.processSchemaRef(mediaType.Schema)
			fmt.Printf("processOperation: 完成处理媒体类型 %s 的Schema\n", mediaTypeKey)
		}
	}
	
	fmt.Printf("processOperation: 开始处理RequestBody\n")
	requestBody := op.RequestBody
	if requestBody != nil && requestBody.Ref == "" {
		fmt.Printf("processOperation: RequestBody存在且无引用，处理Content\n")
		content := requestBody.Content
		fmt.Printf("processOperation: RequestBody Content共%d个媒体类型\n", len(content))
		for mediaTypeKey, mediaType := range content {
			fmt.Printf("processOperation: 处理RequestBody媒体类型 %s\n", mediaTypeKey)
			if mediaType == nil {
				fmt.Printf("processOperation: RequestBody媒体类型 %s 为nil\n", mediaTypeKey)
				continue
			}
			fmt.Printf("processOperation: 开始处理RequestBody媒体类型 %s 的Schema\n", mediaTypeKey)
			mediaType.Schema = s.processSchemaRef(mediaType.Schema)
			fmt.Printf("processOperation: 完成处理RequestBody媒体类型 %s 的Schema\n", mediaTypeKey)
		}
	} else {
		fmt.Printf("processOperation: RequestBody为nil或有引用，跳过处理\n")
	}
	
	fmt.Printf("processOperation: Operation处理完成\n")
}

func (s *schemaNormalizer) process(ref *Schema, args []*Schema) *Schema {
	schemaRef := Unref(s.doc, ref)
	res := schemaRef.Clone()
	res.SpecializedFromGeneric = true
	schema := res
	ext := schema.ExtendedTypeInfo
	specificTypeKey := s.modelKey(ref.GetKey(), args)
	resRef := RefComponentSchemas(specificTypeKey)
	if ref.Ref != "" {
		_, exists := s.doc.Components.Schemas[specificTypeKey]
		if exists {
			return resRef
		}
		res.ExtendedTypeInfo = NewSpecificExtendType(ref, args...)
		s.doc.Components.Schemas[specificTypeKey] = res
	}

	if ext != nil {
		switch ext.Type {
		case ExtendedTypeSpecific:
			return s.process(ext.SpecificType.Type, s.mergeArgs(ext.SpecificType.Args, args))
		case ExtendedTypeParam:
			arg := args[ext.TypeParam.Index]
			if arg == nil {
				return nil
			}
			res = arg
		}
	}

	if schema.Items != nil {
		res.Items = s.process(res.Items, args)
	}
	if schema.AdditionalProperties != nil {
		res.AdditionalProperties = s.process(res.AdditionalProperties, args)
	}
	for key, property := range schema.Properties {
		schema.Properties[key] = s.process(property, args)
	}
	if ref.Ref != "" {
		return resRef
	}
	return res
}

func (s *schemaNormalizer) mergeArgs(args []*Schema, args2 []*Schema) []*Schema {
	res := make([]*Schema, 0, len(args))
	for _, _arg := range args {
		arg := _arg
		ext := arg.ExtendedTypeInfo
		if ext != nil && ext.Type == ExtendedTypeParam {
			arg = args2[ext.TypeParam.Index]
		}
		res = append(res, arg)
	}
	return res
}

func (s *schemaNormalizer) modelKey(key string, args []*Schema) string {
	sb := strings.Builder{}
	sb.WriteString(key)
	if len(args) <= 0 {
		return key
	}
	sb.WriteString("[" + args[0].GetKey())
	for _, ref := range args[1:] {
		sb.WriteString("," + ref.GetKey())
	}
	sb.WriteString("]")
	return sb.String()
}
