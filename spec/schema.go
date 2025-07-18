package spec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"unicode/utf16"

	"github.com/chenwei67/eapi/utils"
	"github.com/go-openapi/jsonpointer"
	"github.com/mohae/deepcopy"
	"github.com/spf13/cast"

	"github.com/getkin/kin-openapi/jsoninfo"
)

const (
	TypeArray   = "array"
	TypeBoolean = "boolean"
	TypeInteger = "integer"
	TypeNumber  = "number"
	TypeObject  = "object"
	TypeString  = "string"

	// constants for integer formats
	formatMinInt32 = float64(math.MinInt32)
	formatMaxInt32 = float64(math.MaxInt32)
	formatMinInt64 = float64(math.MinInt64)
	formatMaxInt64 = float64(math.MaxInt64)
)

var (
	// SchemaErrorDetailsDisabled disables printing of details about schema errors.
	SchemaErrorDetailsDisabled = false

	errSchema = errors.New("input does not match the schema")

	// ErrOneOfConflict is the SchemaError Origin when data matches more than one oneOf schema
	ErrOneOfConflict = errors.New("input matches more than one oneOf schemas")

	// ErrSchemaInputNaN may be returned when validating a number
	ErrSchemaInputNaN = errors.New("floating point NaN is not allowed")
	// ErrSchemaInputInf may be returned when validating a number
	ErrSchemaInputInf = errors.New("floating point Inf is not allowed")
)

// Float64Ptr is a helper for defining OpenAPI schemas.
func Float64Ptr(value float64) *float64 {
	return &value
}

// BoolPtr is a helper for defining OpenAPI schemas.
func BoolPtr(value bool) *bool {
	return &value
}

// Int64Ptr is a helper for defining OpenAPI schemas.
func Int64Ptr(value int64) *int64 {
	return &value
}

// Uint64Ptr is a helper for defining OpenAPI schemas.
func Uint64Ptr(value uint64) *uint64 {
	return &value
}

type Schemas map[string]*Schema

var _ jsonpointer.JSONPointable = (*Schemas)(nil)

// JSONLookup implements github.com/go-openapi/jsonpointer#JSONPointable
func (s Schemas) JSONLookup(token string) (interface{}, error) {
	ref, ok := s[token]
	if ref == nil || ok == false {
		return nil, fmt.Errorf("object has no field %q", token)
	}

	if ref.Ref != "" {
		return &Ref{Ref: ref.Ref}, nil
	}
	return ref, nil
}

type SchemaRefs []*Schema

var _ jsonpointer.JSONPointable = (*Schemas)(nil)

// JSONLookup implements github.com/go-openapi/jsonpointer#JSONPointable
func (s SchemaRefs) JSONLookup(token string) (interface{}, error) {
	i, err := strconv.ParseUint(token, 10, 64)
	if err != nil {
		return nil, err
	}

	if i >= uint64(len(s)) {
		return nil, fmt.Errorf("index out of range: %d", i)
	}

	ref := s[i]

	if ref == nil || ref.Ref != "" {
		return &Ref{Ref: ref.Ref}, nil
	}
	return ref, nil
}

// Schema is specified by OpenAPI/Swagger 3.0 standard.
// See https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.0.3.md#schemaObject
type Schema struct {
	ExtensionProps `json:"-" yaml:"-"`

	Ref          string        `json:"ref,omitempty"`
	Summary      string        `json:"summary,omitempty"`
	OneOf        SchemaRefs    `json:"oneOf,omitempty" yaml:"oneOf,omitempty"`
	AnyOf        SchemaRefs    `json:"anyOf,omitempty" yaml:"anyOf,omitempty"`
	AllOf        SchemaRefs    `json:"allOf,omitempty" yaml:"allOf,omitempty"`
	Not          *Schema       `json:"not,omitempty" yaml:"not,omitempty"`
	Type         string        `json:"type,omitempty" yaml:"type,omitempty"`
	Title        string        `json:"title,omitempty" yaml:"title,omitempty"`
	Format       string        `json:"format,omitempty" yaml:"format,omitempty"`
	Description  string        `json:"description,omitempty" yaml:"description,omitempty"`
	Enum         []interface{} `json:"enum,omitempty" yaml:"enum,omitempty"`
	Default      interface{}   `json:"default,omitempty" yaml:"default,omitempty"`
	Example      interface{}   `json:"example,omitempty" yaml:"example,omitempty"`
	ExternalDocs *ExternalDocs `json:"externalDocs,omitempty" yaml:"externalDocs,omitempty"`

	// Array-related, here for struct compactness
	UniqueItems bool `json:"uniqueItems,omitempty" yaml:"uniqueItems,omitempty"`
	// Number-related, here for struct compactness
	ExclusiveMin bool `json:"exclusiveMinimum,omitempty" yaml:"exclusiveMinimum,omitempty"`
	ExclusiveMax bool `json:"exclusiveMaximum,omitempty" yaml:"exclusiveMaximum,omitempty"`
	// Properties
	Nullable        bool `json:"nullable,omitempty" yaml:"nullable,omitempty"`
	ReadOnly        bool `json:"readOnly,omitempty" yaml:"readOnly,omitempty"`
	WriteOnly       bool `json:"writeOnly,omitempty" yaml:"writeOnly,omitempty"`
	AllowEmptyValue bool `json:"allowEmptyValue,omitempty" yaml:"allowEmptyValue,omitempty"`
	Deprecated      bool `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	XML             *XML `json:"xml,omitempty" yaml:"xml,omitempty"`

	// Number
	Min        *float64 `json:"minimum,omitempty" yaml:"minimum,omitempty"`
	Max        *float64 `json:"maximum,omitempty" yaml:"maximum,omitempty"`
	MultipleOf *float64 `json:"multipleOf,omitempty" yaml:"multipleOf,omitempty"`

	// String
	MinLength       uint64  `json:"minLength,omitempty" yaml:"minLength,omitempty"`
	MaxLength       *uint64 `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	Pattern         string  `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	compiledPattern *regexp.Regexp

	// Array
	MinItems uint64  `json:"minItems,omitempty" yaml:"minItems,omitempty"`
	MaxItems *uint64 `json:"maxItems,omitempty" yaml:"maxItems,omitempty"`
	Items    *Schema `json:"items,omitempty" yaml:"items,omitempty"`

	// Object
	Required                    []string       `json:"required,omitempty" yaml:"required,omitempty"`
	Properties                  Schemas        `json:"properties,omitempty" yaml:"properties,omitempty"`
	MinProps                    uint64         `json:"minProperties,omitempty" yaml:"minProperties,omitempty"`
	MaxProps                    *uint64        `json:"maxProperties,omitempty" yaml:"maxProperties,omitempty"`
	AdditionalPropertiesAllowed *bool          `multijson:"additionalProperties,omitempty" json:"-" yaml:"-"` // In this order...
	AdditionalProperties        *Schema        `multijson:"additionalProperties,omitempty" json:"-" yaml:"-"` // ...for multijson
	Discriminator               *Discriminator `json:"discriminator,omitempty" yaml:"discriminator,omitempty"`

	// 拓展类型信息. 用于代码生成
	ExtendedTypeInfo       *ExtendedTypeInfo `json:"ext,omitempty" yaml:"-"`
	Key                    string            `json:"-" yaml:"-"`
	SpecializedFromGeneric bool              `json:"-"`
}

var _ jsonpointer.JSONPointable = (*Schema)(nil)

func NewSchema() *Schema {
	return &Schema{}
}

func (schema *Schema) WithExtendedType(t *ExtendedTypeInfo) *Schema {
	schema.ExtendedTypeInfo = t
	return schema
}

// MarshalJSON returns the JSON encoding of Schema.
func (schema *Schema) MarshalJSON() ([]byte, error) {
	if schema.Ref != "" {
		return json.Marshal(schemaRef{
			Summary:     schema.Summary,
			Description: schema.Description,
			Ref:         schema.Ref,
		})
	}

	schema = schema.Clone()
	ext := schema.ExtendedTypeInfo
	if ext != nil && len(ext.EnumItems) > 0 {
		desc := schema.Description
		if desc != "" {
			desc += "\n\n"
		}
		desc += "<table><tr><th>Value</th><th>Key</th><th>Description</th></tr>"
		for _, item := range ext.EnumItems {
			desc += fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td></tr>", cast.ToString(item), item.Key, item.Description)
		}
		desc += "</table>"
		schema.Description = desc
	}
	if !utils.Debug() {
		schema.ExtendedTypeInfo = nil
	}
	return jsoninfo.MarshalStrictStruct(schema)
}

// UnmarshalJSON sets Schema to a copy of data.
func (schema *Schema) UnmarshalJSON(data []byte) error {
	return jsoninfo.UnmarshalStrictStruct(data, schema)
}

// JSONLookup implements github.com/go-openapi/jsonpointer#JSONPointable
func (schema *Schema) JSONLookup(token string) (interface{}, error) {
	switch token {
	case "additionalProperties":
		if schema.AdditionalProperties != nil {
			if schema.AdditionalProperties.Ref != "" {
				return &Ref{Ref: schema.AdditionalProperties.Ref}, nil
			}
			return schema.AdditionalProperties, nil
		}
	case "not":
		if schema.Not != nil {
			if schema.Not.Ref != "" {
				return &Ref{Ref: schema.Not.Ref}, nil
			}
			return schema.Not, nil
		}
	case "items":
		if schema.Items != nil {
			if schema.Items.Ref != "" {
				return &Ref{Ref: schema.Items.Ref}, nil
			}
			return schema.Items, nil
		}
	case "oneOf":
		return schema.OneOf, nil
	case "anyOf":
		return schema.AnyOf, nil
	case "allOf":
		return schema.AllOf, nil
	case "type":
		return schema.Type, nil
	case "title":
		return schema.Title, nil
	case "format":
		return schema.Format, nil
	case "description":
		return schema.Description, nil
	case "enum":
		return schema.Enum, nil
	case "default":
		return schema.Default, nil
	case "example":
		return schema.Example, nil
	case "externalDocs":
		return schema.ExternalDocs, nil
	case "additionalPropertiesAllowed":
		return schema.AdditionalPropertiesAllowed, nil
	case "uniqueItems":
		return schema.UniqueItems, nil
	case "exclusiveMin":
		return schema.ExclusiveMin, nil
	case "exclusiveMax":
		return schema.ExclusiveMax, nil
	case "nullable":
		return schema.Nullable, nil
	case "readOnly":
		return schema.ReadOnly, nil
	case "writeOnly":
		return schema.WriteOnly, nil
	case "allowEmptyValue":
		return schema.AllowEmptyValue, nil
	case "xml":
		return schema.XML, nil
	case "deprecated":
		return schema.Deprecated, nil
	case "min":
		return schema.Min, nil
	case "max":
		return schema.Max, nil
	case "multipleOf":
		return schema.MultipleOf, nil
	case "minLength":
		return schema.MinLength, nil
	case "maxLength":
		return schema.MaxLength, nil
	case "pattern":
		return schema.Pattern, nil
	case "minItems":
		return schema.MinItems, nil
	case "maxItems":
		return schema.MaxItems, nil
	case "required":
		return schema.Required, nil
	case "properties":
		return schema.Properties, nil
	case "minProps":
		return schema.MinProps, nil
	case "maxProps":
		return schema.MaxProps, nil
	case "discriminator":
		return schema.Discriminator, nil
	}

	v, _, err := jsonpointer.GetForToken(schema.ExtensionProps, token)
	return v, err
}

func (schema *Schema) NewRef() *Schema {
	// TODO
	return schema
}

func NewTypeParamSchema(param *TypeParam) *Schema {
	return &Schema{
		Type:             "typeParam",
		ExtendedTypeInfo: NewTypeParamExtendedType(param),
	}
}

func NewOneOfSchema(schemas ...*Schema) *Schema {
	return &Schema{
		OneOf: schemas,
	}
}

func NewAnyOfSchema(schemas ...*Schema) *Schema {
	return &Schema{
		AnyOf: schemas,
	}
}

func NewAllOfSchema(schemas ...*Schema) *Schema {
	return &Schema{
		AllOf: schemas,
	}
}

func NewBoolSchema() *Schema {
	return &Schema{
		Type: TypeBoolean,
	}
}

func NewFloat64Schema() *Schema {
	return &Schema{
		Type: TypeNumber,
	}
}

func NewIntegerSchema() *Schema {
	return &Schema{
		Type: TypeInteger,
	}
}

func NewInt32Schema() *Schema {
	return &Schema{
		Type:   TypeInteger,
		Format: "int32",
	}
}

func NewInt64Schema() *Schema {
	return &Schema{
		Type:   TypeInteger,
		Format: "int64",
	}
}

func NewStringSchema() *Schema {
	return &Schema{
		Type: TypeString,
	}
}

func NewDateTimeSchema() *Schema {
	return &Schema{
		Type:   TypeString,
		Format: "date-time",
	}
}

func NewUUIDSchema() *Schema {
	return &Schema{
		Type:   TypeString,
		Format: "uuid",
	}
}

func NewBytesSchema() *Schema {
	return &Schema{
		Type:   TypeString,
		Format: "byte",
	}
}

func NewArraySchema(item *Schema) *Schema {
	return &Schema{
		Type:             TypeArray,
		Items:            item,
		ExtendedTypeInfo: NewArrayExtType(item),
	}
}

func NewObjectSchema() *Schema {
	return &Schema{
		Type:             TypeObject,
		Properties:       make(Schemas),
		ExtendedTypeInfo: NewObjectExtType(),
	}
}

func (schema *Schema) WithNullable() *Schema {
	schema.Nullable = true
	return schema
}

func (schema *Schema) WithMin(value float64) *Schema {
	schema.Min = &value
	return schema
}

func (schema *Schema) WithMax(value float64) *Schema {
	schema.Max = &value
	return schema
}

func (schema *Schema) WithExclusiveMin(value bool) *Schema {
	schema.ExclusiveMin = value
	return schema
}

func (schema *Schema) WithExclusiveMax(value bool) *Schema {
	schema.ExclusiveMax = value
	return schema
}

func (schema *Schema) WithEnum(values ...interface{}) *Schema {
	schema.Enum = values
	return schema
}

func (schema *Schema) WithDefault(defaultValue interface{}) *Schema {
	schema.Default = defaultValue
	return schema
}

func (schema *Schema) WithFormat(value string) *Schema {
	schema.Format = value
	return schema
}

func (schema *Schema) WithLength(i int64) *Schema {
	n := uint64(i)
	schema.MinLength = n
	schema.MaxLength = &n
	return schema
}

func (schema *Schema) WithMinLength(i int64) *Schema {
	n := uint64(i)
	schema.MinLength = n
	return schema
}

func (schema *Schema) WithMaxLength(i int64) *Schema {
	n := uint64(i)
	schema.MaxLength = &n
	return schema
}

func (schema *Schema) WithLengthDecodedBase64(i int64) *Schema {
	n := uint64(i)
	v := (n*8 + 5) / 6
	schema.MinLength = v
	schema.MaxLength = &v
	return schema
}

func (schema *Schema) WithMinLengthDecodedBase64(i int64) *Schema {
	n := uint64(i)
	schema.MinLength = (n*8 + 5) / 6
	return schema
}

func (schema *Schema) WithMaxLengthDecodedBase64(i int64) *Schema {
	n := uint64(i)
	schema.MinLength = (n*8 + 5) / 6
	return schema
}

func (schema *Schema) WithPattern(pattern string) *Schema {
	schema.Pattern = pattern
	schema.compiledPattern = nil
	return schema
}

func (schema *Schema) WithItems(value *Schema) *Schema {
	schema.Items = value
	schema.ExtendedTypeInfo.Items = schema.Items
	return schema
}

func (schema *Schema) WithMinItems(i int64) *Schema {
	n := uint64(i)
	schema.MinItems = n
	return schema
}

func (schema *Schema) WithMaxItems(i int64) *Schema {
	n := uint64(i)
	schema.MaxItems = &n
	return schema
}

func (schema *Schema) WithUniqueItems(unique bool) *Schema {
	schema.UniqueItems = unique
	return schema
}

func (schema *Schema) WithProperty(name string, propertySchema *Schema) *Schema {
	return schema.WithPropertyRef(name, propertySchema)
}

func (schema *Schema) WithPropertyRef(name string, ref *Schema) *Schema {
	properties := schema.Properties
	if properties == nil {
		properties = make(Schemas)
		schema.Properties = properties
	}
	properties[name] = ref
	return schema
}

func (schema *Schema) WithProperties(properties map[string]*Schema) *Schema {
	result := make(Schemas, len(properties))
	for k, v := range properties {
		result[k] = v
	}
	schema.Properties = result
	return schema
}

func (schema *Schema) WithMinProperties(i int64) *Schema {
	n := uint64(i)
	schema.MinProps = n
	return schema
}

func (schema *Schema) WithMaxProperties(i int64) *Schema {
	n := uint64(i)
	schema.MaxProps = &n
	return schema
}

func (schema *Schema) WithAnyAdditionalProperties() *Schema {
	schema.AdditionalProperties = nil
	t := true
	schema.AdditionalPropertiesAllowed = &t
	return schema
}

func (schema *Schema) WithAdditionalProperties(v *Schema) *Schema {
	schema.AdditionalProperties = v
	return schema
}

func (schema *Schema) IsEmpty() bool {
	if schema.Type != "" || schema.Format != "" || len(schema.Enum) != 0 ||
		schema.UniqueItems || schema.ExclusiveMin || schema.ExclusiveMax ||
		schema.Nullable || schema.ReadOnly || schema.WriteOnly || schema.AllowEmptyValue ||
		schema.Min != nil || schema.Max != nil || schema.MultipleOf != nil ||
		schema.MinLength != 0 || schema.MaxLength != nil || schema.Pattern != "" ||
		schema.MinItems != 0 || schema.MaxItems != nil ||
		len(schema.Required) != 0 ||
		schema.MinProps != 0 || schema.MaxProps != nil {
		return false
	}
	if n := schema.Not; n != nil && !n.IsEmpty() {
		return false
	}
	if ap := schema.AdditionalProperties; ap != nil && !ap.IsEmpty() {
		return false
	}
	if apa := schema.AdditionalPropertiesAllowed; apa != nil && !*apa {
		return false
	}
	if items := schema.Items; items != nil && !items.IsEmpty() {
		return false
	}
	for _, s := range schema.Properties {
		if !s.IsEmpty() {
			return false
		}
	}
	for _, s := range schema.OneOf {
		if !s.IsEmpty() {
			return false
		}
	}
	for _, s := range schema.AnyOf {
		if !s.IsEmpty() {
			return false
		}
	}
	for _, s := range schema.AllOf {
		if !s.IsEmpty() {
			return false
		}
	}
	return true
}

// Validate returns an error if Schema does not comply with the OpenAPI spec.
func (schema *Schema) Validate(ctx context.Context) error {
	return schema.validate(ctx, []*Schema{})
}

func (schema *Schema) validate(ctx context.Context, stack []*Schema) (err error) {
	validationOpts := getValidationOptions(ctx)

	for _, existing := range stack {
		if existing == schema {
			return
		}
	}
	stack = append(stack, schema)

	if schema.ReadOnly && schema.WriteOnly {
		return errors.New("a property MUST NOT be marked as both readOnly and writeOnly being true")
	}

	for _, item := range schema.OneOf {
		v := item
		if v == nil {
			return foundUnresolvedRef(item.Ref)
		}
		if err = v.validate(ctx, stack); err == nil {
			return
		}
	}

	for _, item := range schema.AnyOf {
		v := item
		if v == nil {
			return foundUnresolvedRef(item.Ref)
		}
		if err = v.validate(ctx, stack); err != nil {
			return
		}
	}

	for _, item := range schema.AllOf {
		v := item
		if v == nil {
			return foundUnresolvedRef(item.Ref)
		}
		if err = v.validate(ctx, stack); err != nil {
			return
		}
	}

	if ref := schema.Not; ref != nil {
		v := ref
		if v == nil {
			return foundUnresolvedRef(ref.Ref)
		}
		if err = v.validate(ctx, stack); err != nil {
			return
		}
	}

	schemaType := schema.Type
	switch schemaType {
	case "":
	case TypeBoolean:
	case TypeNumber:
		if format := schema.Format; len(format) > 0 {
			switch format {
			case "float", "double":
			default:
				if validationOpts.SchemaFormatValidationEnabled {
					return unsupportedFormat(format)
				}
			}
		}
	case TypeInteger:
		if format := schema.Format; len(format) > 0 {
			switch format {
			case "int32", "int64":
			default:
				if validationOpts.SchemaFormatValidationEnabled {
					return unsupportedFormat(format)
				}
			}
		}
	case TypeString:
		if format := schema.Format; len(format) > 0 {
			switch format {
			// Supported by OpenAPIv3.0.3:
			// https://openapis.org/oas/v3.0.3
			case "byte", "binary", "date", "date-time", "password":
			// In JSON Draft-07 (not validated yet though):
			// https://json-schema.org/draft-07/json-schema-release-notes.html#formats
			case "iri", "iri-reference", "uri-template", "idn-email", "idn-hostname":
			case "json-pointer", "relative-json-pointer", "regex", "time":
			// In JSON Draft 2019-09 (not validated yet though):
			// https://json-schema.org/draft/2019-09/release-notes.html#format-vocabulary
			case "duration", "uuid":
			// Defined in some other specification
			case "email", "hostname", "ipv4", "ipv6", "uri", "uri-reference":
			default:
				// Try to check for custom defined formats
				if _, ok := SchemaStringFormats[format]; !ok && validationOpts.SchemaFormatValidationEnabled {
					return unsupportedFormat(format)
				}
			}
		}
		if schema.Pattern != "" && !validationOpts.SchemaPatternValidationDisabled {
			if err = schema.compilePattern(); err != nil {
				return err
			}
		}
	case TypeArray:
		if schema.Items == nil {
			return errors.New("when schema type is 'array', schema 'items' must be non-null")
		}
	case TypeObject:
	default:
		return fmt.Errorf("unsupported 'type' value %q", schemaType)
	}

	if ref := schema.Items; ref != nil {
		v := ref
		if v == nil {
			return foundUnresolvedRef(ref.Ref)
		}
		if err = v.validate(ctx, stack); err != nil {
			return
		}
	}

	properties := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		properties = append(properties, name)
	}
	sort.Strings(properties)
	for _, name := range properties {
		ref := schema.Properties[name]
		v := ref
		if v == nil {
			return foundUnresolvedRef(ref.Ref)
		}
		if err = v.validate(ctx, stack); err != nil {
			return
		}
	}

	if ref := schema.AdditionalProperties; ref != nil {
		v := ref
		if v == nil {
			return foundUnresolvedRef(ref.Ref)
		}
		if err = v.validate(ctx, stack); err != nil {
			return
		}
	}

	if v := schema.ExternalDocs; v != nil {
		if err = v.Validate(ctx); err != nil {
			return fmt.Errorf("invalid external docs: %w", err)
		}
	}

	if v := schema.Default; v != nil {
		if err := schema.VisitJSON(v); err != nil {
			return fmt.Errorf("invalid default: %w", err)
		}
	}

	if x := schema.Example; x != nil && !validationOpts.ExamplesValidationDisabled {
		if err := validateExampleValue(ctx, x, schema); err != nil {
			return fmt.Errorf("invalid example: %w", err)
		}
	}

	return
}

func (schema *Schema) IsMatching(value interface{}) bool {
	settings := newSchemaValidationSettings(FailFast())
	return schema.visitJSON(settings, value) == nil
}

func (schema *Schema) IsMatchingJSONBoolean(value bool) bool {
	settings := newSchemaValidationSettings(FailFast())
	return schema.visitJSON(settings, value) == nil
}

func (schema *Schema) IsMatchingJSONNumber(value float64) bool {
	settings := newSchemaValidationSettings(FailFast())
	return schema.visitJSON(settings, value) == nil
}

func (schema *Schema) IsMatchingJSONString(value string) bool {
	settings := newSchemaValidationSettings(FailFast())
	return schema.visitJSON(settings, value) == nil
}

func (schema *Schema) IsMatchingJSONArray(value []interface{}) bool {
	settings := newSchemaValidationSettings(FailFast())
	return schema.visitJSON(settings, value) == nil
}

func (schema *Schema) IsMatchingJSONObject(value map[string]interface{}) bool {
	settings := newSchemaValidationSettings(FailFast())
	return schema.visitJSON(settings, value) == nil
}

func (schema *Schema) VisitJSON(value interface{}, opts ...SchemaValidationOption) error {
	settings := newSchemaValidationSettings(opts...)
	return schema.visitJSON(settings, value)
}

func (schema *Schema) visitJSON(settings *schemaValidationSettings, value interface{}) (err error) {
	switch value := value.(type) {
	case nil:
		return schema.visitJSONNull(settings)
	case float64:
		if math.IsNaN(value) {
			return ErrSchemaInputNaN
		}
		if math.IsInf(value, 0) {
			return ErrSchemaInputInf
		}
	}

	if schema.IsEmpty() {
		return
	}
	if err = schema.visitSetOperations(settings, value); err != nil {
		return
	}

	switch value := value.(type) {
	case bool:
		return schema.visitJSONBoolean(settings, value)
	case int:
		return schema.visitJSONNumber(settings, float64(value))
	case int32:
		return schema.visitJSONNumber(settings, float64(value))
	case int64:
		return schema.visitJSONNumber(settings, float64(value))
	case float64:
		return schema.visitJSONNumber(settings, value)
	case string:
		return schema.visitJSONString(settings, value)
	case []interface{}:
		return schema.visitJSONArray(settings, value)
	case map[string]interface{}:
		return schema.visitJSONObject(settings, value)
	case map[interface{}]interface{}: // for YAML cf. issue #444
		values := make(map[string]interface{}, len(value))
		for key, v := range value {
			if k, ok := key.(string); ok {
				values[k] = v
			}
		}
		if len(value) == len(values) {
			return schema.visitJSONObject(settings, values)
		}
	}
	return &SchemaError{
		Value:                 value,
		Schema:                schema,
		SchemaField:           "type",
		Reason:                fmt.Sprintf("unhandled value of type %T", value),
		customizeMessageError: settings.customizeMessageError,
	}
}

func (schema *Schema) visitSetOperations(settings *schemaValidationSettings, value interface{}) (err error) {
	if enum := schema.Enum; len(enum) != 0 {
		for _, v := range enum {
			if reflect.DeepEqual(v, value) {
				return
			}
		}
		if settings.failfast {
			return errSchema
		}
		return &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "enum",
			Reason:                "value is not one of the allowed values",
			customizeMessageError: settings.customizeMessageError,
		}
	}

	if ref := schema.Not; ref != nil {
		v := ref
		if v == nil {
			return foundUnresolvedRef(ref.Ref)
		}
		if err := v.visitJSON(settings, value); err == nil {
			if settings.failfast {
				return errSchema
			}
			return &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "not",
				customizeMessageError: settings.customizeMessageError,
			}
		}
	}

	if v := schema.OneOf; len(v) > 0 {
		var discriminatorRef string
		if schema.Discriminator != nil {
			pn := schema.Discriminator.PropertyName
			if valuemap, okcheck := value.(map[string]interface{}); okcheck {
				discriminatorVal, okcheck := valuemap[pn]
				if !okcheck {
					return errors.New("input does not contain the discriminator property")
				}

				discriminatorValString, okcheck := discriminatorVal.(string)
				if !okcheck {
					return errors.New("descriminator value is not a string")
				}

				if discriminatorRef, okcheck = schema.Discriminator.Mapping[discriminatorValString]; len(schema.Discriminator.Mapping) > 0 && !okcheck {
					return errors.New("input does not contain a valid discriminator value")
				}
			}
		}

		var (
			ok               = 0
			validationErrors = multiErrorForOneOf{}
			matchedOneOfIdx  = 0
			tempValue        = value
		)
		// make a deep copy to protect origin value from being injected default value that defined in mismatched oneOf schema
		if settings.asreq || settings.asrep {
			tempValue = deepcopy.Copy(value)
		}
		for idx, item := range v {
			v := item
			if v == nil {
				return foundUnresolvedRef(item.Ref)
			}

			if discriminatorRef != "" && discriminatorRef != item.Ref {
				continue
			}

			if err := v.visitJSON(settings, tempValue); err != nil {
				validationErrors = append(validationErrors, err)
				continue
			}

			matchedOneOfIdx = idx
			ok++
		}

		if ok != 1 {
			if len(validationErrors) > 1 {
				return fmt.Errorf("doesn't match schema due to: %w", validationErrors)
			}
			if settings.failfast {
				return errSchema
			}
			e := &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "oneOf",
				customizeMessageError: settings.customizeMessageError,
			}
			if ok > 1 {
				e.Origin = ErrOneOfConflict
			} else if len(validationErrors) == 1 {
				e.Origin = validationErrors[0]
			}

			return e
		}

		if settings.asreq || settings.asrep {
			_ = v[matchedOneOfIdx].visitJSON(settings, value)
		}
	}

	if v := schema.AnyOf; len(v) > 0 {
		var (
			ok              = false
			matchedAnyOfIdx = 0
			tempValue       = value
		)
		// make a deep copy to protect origin value from being injected default value that defined in mismatched anyOf schema
		if settings.asreq || settings.asrep {
			tempValue = deepcopy.Copy(value)
		}
		for idx, item := range v {
			v := item
			if v == nil {
				return foundUnresolvedRef(item.Ref)
			}
			if err := v.visitJSON(settings, tempValue); err == nil {
				ok = true
				matchedAnyOfIdx = idx
				break
			}
		}
		if !ok {
			if settings.failfast {
				return errSchema
			}
			return &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "anyOf",
				customizeMessageError: settings.customizeMessageError,
			}
		}

		_ = v[matchedAnyOfIdx].visitJSON(settings, value)
	}

	for _, item := range schema.AllOf {
		v := item
		if v == nil {
			return foundUnresolvedRef(item.Ref)
		}
		if err := v.visitJSON(settings, value); err != nil {
			if settings.failfast {
				return errSchema
			}
			return &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "allOf",
				Origin:                err,
				customizeMessageError: settings.customizeMessageError,
			}
		}
	}
	return
}

func (schema *Schema) visitJSONNull(settings *schemaValidationSettings) (err error) {
	if schema.Nullable {
		return
	}
	if settings.failfast {
		return errSchema
	}
	return &SchemaError{
		Value:                 nil,
		Schema:                schema,
		SchemaField:           "nullable",
		Reason:                "Value is not nullable",
		customizeMessageError: settings.customizeMessageError,
	}
}

func (schema *Schema) VisitJSONBoolean(value bool) error {
	settings := newSchemaValidationSettings()
	return schema.visitJSONBoolean(settings, value)
}

func (schema *Schema) visitJSONBoolean(settings *schemaValidationSettings, value bool) (err error) {
	if schemaType := schema.Type; schemaType != "" && schemaType != TypeBoolean {
		return schema.expectedType(settings, TypeBoolean)
	}
	return
}

func (schema *Schema) VisitJSONNumber(value float64) error {
	settings := newSchemaValidationSettings()
	return schema.visitJSONNumber(settings, value)
}

func (schema *Schema) visitJSONNumber(settings *schemaValidationSettings, value float64) error {
	var me MultiError
	schemaType := schema.Type
	if schemaType == TypeInteger {
		if bigFloat := big.NewFloat(value); !bigFloat.IsInt() {
			if settings.failfast {
				return errSchema
			}
			err := &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "type",
				Reason:                "Value must be an integer",
				customizeMessageError: settings.customizeMessageError,
			}
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
	} else if schemaType != "" && schemaType != TypeNumber {
		return schema.expectedType(settings, "number, integer")
	}

	// formats
	if schemaType == TypeInteger && schema.Format != "" {
		formatMin := float64(0)
		formatMax := float64(0)
		switch schema.Format {
		case "int32":
			formatMin = formatMinInt32
			formatMax = formatMaxInt32
		case "int64":
			formatMin = formatMinInt64
			formatMax = formatMaxInt64
		default:
			if settings.formatValidationEnabled {
				return unsupportedFormat(schema.Format)
			}
		}
		if formatMin != 0 && formatMax != 0 && !(formatMin <= value && value <= formatMax) {
			if settings.failfast {
				return errSchema
			}
			err := &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "format",
				Reason:                fmt.Sprintf("number must be an %s", schema.Format),
				customizeMessageError: settings.customizeMessageError,
			}
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
	}

	// "exclusiveMinimum"
	if v := schema.ExclusiveMin; v && !(*schema.Min < value) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "exclusiveMinimum",
			Reason:                fmt.Sprintf("number must be more than %g", *schema.Min),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "exclusiveMaximum"
	if v := schema.ExclusiveMax; v && !(*schema.Max > value) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "exclusiveMaximum",
			Reason:                fmt.Sprintf("number must be less than %g", *schema.Max),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "minimum"
	if v := schema.Min; v != nil && !(*v <= value) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "minimum",
			Reason:                fmt.Sprintf("number must be at least %g", *v),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "maximum"
	if v := schema.Max; v != nil && !(*v >= value) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "maximum",
			Reason:                fmt.Sprintf("number must be at most %g", *v),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "multipleOf"
	if v := schema.MultipleOf; v != nil {
		// "A numeric instance is valid only if division by this keyword's
		//    value results in an integer."
		if bigFloat := big.NewFloat(value / *v); !bigFloat.IsInt() {
			if settings.failfast {
				return errSchema
			}
			err := &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "multipleOf",
				customizeMessageError: settings.customizeMessageError,
			}
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
	}

	if len(me) > 0 {
		return me
	}

	return nil
}

func (schema *Schema) VisitJSONString(value string) error {
	settings := newSchemaValidationSettings()
	return schema.visitJSONString(settings, value)
}

func (schema *Schema) visitJSONString(settings *schemaValidationSettings, value string) error {
	if schemaType := schema.Type; schemaType != "" && schemaType != TypeString {
		return schema.expectedType(settings, TypeString)
	}

	var me MultiError

	// "minLength" and "maxLength"
	minLength := schema.MinLength
	maxLength := schema.MaxLength
	if minLength != 0 || maxLength != nil {
		// JSON schema string lengths are UTF-16, not UTF-8!
		length := int64(0)
		for _, r := range value {
			if utf16.IsSurrogate(r) {
				length += 2
			} else {
				length++
			}
		}
		if minLength != 0 && length < int64(minLength) {
			if settings.failfast {
				return errSchema
			}
			err := &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "minLength",
				Reason:                fmt.Sprintf("minimum string length is %d", minLength),
				customizeMessageError: settings.customizeMessageError,
			}
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
		if maxLength != nil && length > int64(*maxLength) {
			if settings.failfast {
				return errSchema
			}
			err := &SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "maxLength",
				Reason:                fmt.Sprintf("maximum string length is %d", *maxLength),
				customizeMessageError: settings.customizeMessageError,
			}
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
	}

	// "pattern"
	if schema.Pattern != "" && schema.compiledPattern == nil && !settings.patternValidationDisabled {
		var err error
		if err = schema.compilePattern(); err != nil {
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
	}
	if cp := schema.compiledPattern; cp != nil && !cp.MatchString(value) {
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "pattern",
			Reason:                fmt.Sprintf(`string doesn't match the regular expression "%s"`, schema.Pattern),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "format"
	var formatStrErr string
	var formatErr error
	if format := schema.Format; format != "" {
		if f, ok := SchemaStringFormats[format]; ok {
			switch {
			case f.regexp != nil && f.callback == nil:
				if cp := f.regexp; !cp.MatchString(value) {
					formatStrErr = fmt.Sprintf(`string doesn't match the format %q (regular expression "%s")`, format, cp.String())
				}
			case f.regexp == nil && f.callback != nil:
				if err := f.callback(value); err != nil {
					formatErr = err
				}
			default:
				formatStrErr = fmt.Sprintf("corrupted entry %q in SchemaStringFormats", format)
			}
		}
	}
	if formatStrErr != "" || formatErr != nil {
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "format",
			Reason:                formatStrErr,
			Origin:                formatErr,
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)

	}

	if len(me) > 0 {
		return me
	}

	return nil
}

func (schema *Schema) VisitJSONArray(value []interface{}) error {
	settings := newSchemaValidationSettings()
	return schema.visitJSONArray(settings, value)
}

func (schema *Schema) visitJSONArray(settings *schemaValidationSettings, value []interface{}) error {
	if schemaType := schema.Type; schemaType != "" && schemaType != TypeArray {
		return schema.expectedType(settings, TypeArray)
	}

	var me MultiError

	lenValue := int64(len(value))

	// "minItems"
	if v := schema.MinItems; v != 0 && lenValue < int64(v) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "minItems",
			Reason:                fmt.Sprintf("minimum number of items is %d", v),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "maxItems"
	if v := schema.MaxItems; v != nil && lenValue > int64(*v) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "maxItems",
			Reason:                fmt.Sprintf("maximum number of items is %d", *v),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "uniqueItems"
	if sliceUniqueItemsChecker == nil {
		sliceUniqueItemsChecker = isSliceOfUniqueItems
	}
	if v := schema.UniqueItems; v && !sliceUniqueItemsChecker(value) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "uniqueItems",
			Reason:                "duplicate items found",
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "items"
	if itemSchemaRef := schema.Items; itemSchemaRef != nil {
		itemSchema := itemSchemaRef
		if itemSchema == nil {
			return foundUnresolvedRef(itemSchemaRef.Ref)
		}
		for i, item := range value {
			if err := itemSchema.visitJSON(settings, item); err != nil {
				err = markSchemaErrorIndex(err, i)
				if !settings.multiError {
					return err
				}
				if itemMe, ok := err.(MultiError); ok {
					me = append(me, itemMe...)
				} else {
					me = append(me, err)
				}
			}
		}
	}

	if len(me) > 0 {
		return me
	}

	return nil
}

func (schema *Schema) VisitJSONObject(value map[string]interface{}) error {
	settings := newSchemaValidationSettings()
	return schema.visitJSONObject(settings, value)
}

func (schema *Schema) visitJSONObject(settings *schemaValidationSettings, value map[string]interface{}) error {
	if schemaType := schema.Type; schemaType != "" && schemaType != TypeObject {
		return schema.expectedType(settings, TypeObject)
	}

	var me MultiError

	if settings.asreq || settings.asrep {
		properties := make([]string, 0, len(schema.Properties))
		for propName := range schema.Properties {
			properties = append(properties, propName)
		}
		sort.Strings(properties)
		for _, propName := range properties {
			propSchema := schema.Properties[propName]
			reqRO := settings.asreq && propSchema.ReadOnly
			repWO := settings.asrep && propSchema.WriteOnly

			if value[propName] == nil {
				if dlft := propSchema.Default; dlft != nil && !reqRO && !repWO {
					value[propName] = dlft
					if f := settings.defaultsSet; f != nil {
						settings.onceSettingDefaults.Do(f)
					}
				}
			}

			if value[propName] != nil {
				if reqRO {
					me = append(me, fmt.Errorf("readOnly property %q in request", propName))
				} else if repWO {
					me = append(me, fmt.Errorf("writeOnly property %q in response", propName))
				}
			}
		}
	}

	// "properties"
	properties := schema.Properties
	lenValue := int64(len(value))

	// "minProperties"
	if v := schema.MinProps; v != 0 && lenValue < int64(v) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "minProperties",
			Reason:                fmt.Sprintf("there must be at least %d properties", v),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "maxProperties"
	if v := schema.MaxProps; v != nil && lenValue > int64(*v) {
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "maxProperties",
			Reason:                fmt.Sprintf("there must be at most %d properties", *v),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "additionalProperties"
	var additionalProperties *Schema
	if ref := schema.AdditionalProperties; ref != nil {
		additionalProperties = ref
	}
	keys := make([]string, 0, len(value))
	for k := range value {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := value[k]
		if properties != nil {
			propertyRef := properties[k]
			if propertyRef != nil {
				p := propertyRef
				if p == nil {
					return foundUnresolvedRef(propertyRef.Ref)
				}
				if err := p.visitJSON(settings, v); err != nil {
					if settings.failfast {
						return errSchema
					}
					err = markSchemaErrorKey(err, k)
					if !settings.multiError {
						return err
					}
					if v, ok := err.(MultiError); ok {
						me = append(me, v...)
						continue
					}
					me = append(me, err)
				}
				continue
			}
		}
		allowed := schema.AdditionalPropertiesAllowed
		if additionalProperties != nil || allowed == nil || *allowed {
			if additionalProperties != nil {
				if err := additionalProperties.visitJSON(settings, v); err != nil {
					if settings.failfast {
						return errSchema
					}
					err = markSchemaErrorKey(err, k)
					if !settings.multiError {
						return err
					}
					if v, ok := err.(MultiError); ok {
						me = append(me, v...)
						continue
					}
					me = append(me, err)
				}
			}
			continue
		}
		if settings.failfast {
			return errSchema
		}
		err := &SchemaError{
			Value:                 value,
			Schema:                schema,
			SchemaField:           "properties",
			Reason:                fmt.Sprintf("property %q is unsupported", k),
			customizeMessageError: settings.customizeMessageError,
		}
		if !settings.multiError {
			return err
		}
		me = append(me, err)
	}

	// "required"
	for _, k := range schema.Required {
		if _, ok := value[k]; !ok {
			if s := schema.Properties[k]; s != nil && s.ReadOnly && settings.asreq {
				continue
			}
			if s := schema.Properties[k]; s != nil && s.WriteOnly && settings.asrep {
				continue
			}
			if settings.failfast {
				return errSchema
			}
			err := markSchemaErrorKey(&SchemaError{
				Value:                 value,
				Schema:                schema,
				SchemaField:           "required",
				Reason:                fmt.Sprintf("property %q is missing", k),
				customizeMessageError: settings.customizeMessageError,
			}, k)
			if !settings.multiError {
				return err
			}
			me = append(me, err)
		}
	}

	if len(me) > 0 {
		return me
	}

	return nil
}

func (schema *Schema) expectedType(settings *schemaValidationSettings, typ string) error {
	if settings.failfast {
		return errSchema
	}
	return &SchemaError{
		Value:                 typ,
		Schema:                schema,
		SchemaField:           "type",
		Reason:                "Field must be set to " + schema.Type + " or not be present",
		customizeMessageError: settings.customizeMessageError,
	}
}

func (schema *Schema) compilePattern() (err error) {
	if schema.compiledPattern, err = regexp.Compile(schema.Pattern); err != nil {
		return &SchemaError{
			Schema:      schema,
			SchemaField: "pattern",
			Reason:      fmt.Sprintf("cannot compile pattern %q: %v", schema.Pattern, err),
		}
	}
	return nil
}

func (schema *Schema) WithType(s string) *Schema {
	schema.Type = s
	return schema
}

func (schema *Schema) WithDescription(s string) *Schema {
	schema.Description = s
	return schema
}

func (schema *Schema) Clone() *Schema {
	ret := deepcopy.Copy(*schema).(Schema)
	return &ret
}

type SchemaError struct {
	Value                 interface{}
	reversePath           []string
	Schema                *Schema
	SchemaField           string
	Reason                string
	Origin                error
	customizeMessageError func(err *SchemaError) string
}

var _ interface{ Unwrap() error } = SchemaError{}

func markSchemaErrorKey(err error, key string) error {
	if v, ok := err.(*SchemaError); ok {
		v.reversePath = append(v.reversePath, key)
		return v
	}
	if v, ok := err.(MultiError); ok {
		for _, e := range v {
			_ = markSchemaErrorKey(e, key)
		}
		return v
	}
	return err
}

func markSchemaErrorIndex(err error, index int) error {
	if v, ok := err.(*SchemaError); ok {
		v.reversePath = append(v.reversePath, strconv.FormatInt(int64(index), 10))
		return v
	}
	if v, ok := err.(MultiError); ok {
		for _, e := range v {
			_ = markSchemaErrorIndex(e, index)
		}
		return v
	}
	return err
}

func (err *SchemaError) JSONPointer() []string {
	reversePath := err.reversePath
	path := append([]string(nil), reversePath...)
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

func (err *SchemaError) Error() string {
	if err.customizeMessageError != nil {
		if msg := err.customizeMessageError(err); msg != "" {
			return msg
		}
	}

	if err.Origin != nil {
		return err.Origin.Error()
	}

	buf := bytes.NewBuffer(make([]byte, 0, 256))
	if len(err.reversePath) > 0 {
		buf.WriteString(`Error at "`)
		reversePath := err.reversePath
		for i := len(reversePath) - 1; i >= 0; i-- {
			buf.WriteByte('/')
			buf.WriteString(reversePath[i])
		}
		buf.WriteString(`": `)
	}
	reason := err.Reason
	if reason == "" {
		buf.WriteString(`Doesn't match schema "`)
		buf.WriteString(err.SchemaField)
		buf.WriteString(`"`)
	} else {
		buf.WriteString(reason)
	}
	if !SchemaErrorDetailsDisabled {
		buf.WriteString("\nSchema:\n  ")
		encoder := json.NewEncoder(buf)
		encoder.SetIndent("  ", "  ")
		if err := encoder.Encode(err.Schema); err != nil {
			panic(err)
		}
		buf.WriteString("\nValue:\n  ")
		if err := encoder.Encode(err); err != nil {
			panic(err)
		}
	}
	return buf.String()
}

func (err SchemaError) Unwrap() error {
	return err.Origin
}

func isSliceOfUniqueItems(xs []interface{}) bool {
	s := len(xs)
	m := make(map[string]struct{}, s)
	for _, x := range xs {
		// The input slice is converted from a JSON string, there shall
		// have no error when covert it back.
		key, _ := json.Marshal(&x)
		m[string(key)] = struct{}{}
	}
	return s == len(m)
}

// SliceUniqueItemsChecker is an function used to check if an given slice
// have unique items.
type SliceUniqueItemsChecker func(items []interface{}) bool

// By default using predefined func isSliceOfUniqueItems which make use of
// json.Marshal to generate a key for map used to check if a given slice
// have unique items.
var sliceUniqueItemsChecker SliceUniqueItemsChecker = isSliceOfUniqueItems

// RegisterArrayUniqueItemsChecker is used to register a customized function
// used to check if JSON array have unique items.
func RegisterArrayUniqueItemsChecker(fn SliceUniqueItemsChecker) {
	sliceUniqueItemsChecker = fn
}

func unsupportedFormat(format string) error {
	return fmt.Errorf("unsupported 'format' value %q", format)
}
