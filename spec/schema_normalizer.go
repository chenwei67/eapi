package spec

import (
	"strings"
)

type schemaNormalizer struct {
	doc *T
}

func newSchemaNormalizer(doc *T) *schemaNormalizer {
	return &schemaNormalizer{doc: doc}
}

func (s *schemaNormalizer) normalize() *T {
	// normalize: 开始规范化处理

	// 处理 Components.Schemas
	// normalize: 开始处理Components.Schemas
	for key, ref := range s.doc.Components.Schemas {
		// normalize: 处理schema
		if ref == nil || ref.Ref != "" {
			// normalize: 跳过schema (ref为空或有引用)
			continue
		}
		ext := ref.ExtendedTypeInfo
		if ext == nil || ext.Type != ExtendedTypeSpecific {
			// normalize: 跳过schema (ExtendedTypeInfo为空或类型不匹配)
			continue
		}

		// normalize: 开始处理特定类型schema
		s.doc.Components.Schemas[key] = s.process(ext.SpecificType.Type, ext.SpecificType.Args)
		// normalize: 完成处理特定类型schema
	}

	// 处理 Paths
	// normalize: 开始处理Paths
	for _, pathItem := range s.doc.Paths {
		// normalize: 开始处理路径
		s.processPathItem(pathItem)
		// normalize: 完成处理路径
	}

	// normalize: 规范化处理完成
	return s.doc
}

func (s *schemaNormalizer) processPathItem(item *PathItem) {
	// processPathItem: 开始处理PathItem
	
	if item == nil {
		// processPathItem: PathItem为nil，跳过处理
		return
	}
	
	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			// processPathItem: 发生panic
			panic(r) // 重新抛出panic
		}
	}()
	
	// processPathItem: 处理Connect操作
	s.processOperation(item.Connect)
	
	// processPathItem: 处理Delete操作
	s.processOperation(item.Delete)
	
	// processPathItem: 处理Get操作
	s.processOperation(item.Get)
	
	// processPathItem: 处理Head操作
	s.processOperation(item.Head)
	
	// processPathItem: 处理Options操作
	s.processOperation(item.Options)
	
	// processPathItem: 处理Patch操作
	s.processOperation(item.Patch)
	
	// processPathItem: 处理Post操作
	s.processOperation(item.Post)
	
	// processPathItem: 处理Put操作
	s.processOperation(item.Put)
	
	// processPathItem: 处理Trace操作
	s.processOperation(item.Trace)
	
	// processPathItem: PathItem处理完成
}

func (s *schemaNormalizer) processSchemaRef(ref *Schema) *Schema {
	// processSchemaRef: 开始处理SchemaRef
	
	if ref == nil {
		// processSchemaRef: Schema为nil，返回nil
		return ref
	}
	
	if ref.Ref != "" {
		// processSchemaRef: Schema有引用，直接返回
		return ref
	}
	
	ext := ref.ExtendedTypeInfo
	if ext == nil {
		// processSchemaRef: ExtendedTypeInfo为nil，直接返回
		return ref
	}
	
	if ext.Type != ExtendedTypeSpecific {
		// processSchemaRef: ExtendedTypeInfo类型不是ExtendedTypeSpecific，直接返回
		return ref
	}
	
	// processSchemaRef: 开始处理特定类型，调用process方法
	
	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			// processSchemaRef: 在process方法中发生panic
			panic(r) // 重新抛出panic
		}
	}()
	
	result := s.process(ext.SpecificType.Type, ext.SpecificType.Args)
	// processSchemaRef: process方法执行完成
	return result
}

func (s *schemaNormalizer) processOperation(op *Operation) {
	// processOperation: 开始处理Operation
	
	if op == nil {
		// processOperation: Operation为nil，跳过处理
		return
	}
	
	// 使用defer来捕获可能的panic
	defer func() {
		if r := recover(); r != nil {
			// processOperation: 发生panic
			panic(r) // 重新抛出panic
		}
	}()
	
	// processOperation: 处理Responses
	for _, ref := range op.Responses {
		// processOperation: 处理响应码
		if ref == nil {
			// processOperation: 响应码的ref为nil
			continue
		}
		// processOperation: 处理响应码的Content
		for _, mediaType := range ref.Content {
			// processOperation: 处理媒体类型
			if mediaType == nil {
				// processOperation: 媒体类型为nil
				continue
			}
			// processOperation: 开始处理媒体类型的Schema
			mediaType.Schema = s.processSchemaRef(mediaType.Schema)
			// processOperation: 完成处理媒体类型的Schema
		}
	}
	
	// processOperation: 开始处理RequestBody
	requestBody := op.RequestBody
	if requestBody != nil && requestBody.Ref == "" {
		// processOperation: RequestBody存在且无引用，处理Content
		content := requestBody.Content
		// processOperation: RequestBody Content
		for _, mediaType := range content {
			// processOperation: 处理RequestBody媒体类型
			if mediaType == nil {
				// processOperation: RequestBody媒体类型为nil
				continue
			}
			// processOperation: 开始处理RequestBody媒体类型的Schema
			mediaType.Schema = s.processSchemaRef(mediaType.Schema)
			// processOperation: 完成处理RequestBody媒体类型的Schema
		}
	} else {
		// processOperation: RequestBody为nil或有引用，跳过处理
	}
	
	// processOperation: Operation处理完成
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
