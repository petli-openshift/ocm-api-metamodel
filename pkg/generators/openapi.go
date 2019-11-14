/*
Copyright (c) 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package generators

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openshift-online/ocm-api-metamodel/pkg/concepts"
	"github.com/openshift-online/ocm-api-metamodel/pkg/golang"
	"github.com/openshift-online/ocm-api-metamodel/pkg/http"
	"github.com/openshift-online/ocm-api-metamodel/pkg/openapi"
	"github.com/openshift-online/ocm-api-metamodel/pkg/reporter"
)

// OpenAPIGeneratorBuilder is an object used to configure and build the OpenAPI specifications
// generator. Don't create instances directly, use the NewOpenAPIGenerator function instead.
type OpenAPIGeneratorBuilder struct {
	reporter *reporter.Reporter
	model    *concepts.Model
	output   string
	names    *openapi.NamesCalculator
	binding  *http.BindingCalculator
	packages *golang.PackagesCalculator
}

// OpenAPIGenerator generates OpenAPI specifications for the model.
type OpenAPIGenerator struct {
	reporter *reporter.Reporter
	errors   int
	model    *concepts.Model
	output   string
	names    *openapi.NamesCalculator
	binding  *http.BindingCalculator
	packages *golang.PackagesCalculator
	buffer   *openapi.Buffer
}

// NewOpenAPIGenerator creates a new builder for OpenAPI specification generators.
func NewOpenAPIGenerator() *OpenAPIGeneratorBuilder {
	return &OpenAPIGeneratorBuilder{}
}

// Reporter sets the object that will be used to report information about the generation process,
// including errors.
func (b *OpenAPIGeneratorBuilder) Reporter(value *reporter.Reporter) *OpenAPIGeneratorBuilder {
	b.reporter = value
	return b
}

// Model sets the model that will be used by the types generator.
func (b *OpenAPIGeneratorBuilder) Model(value *concepts.Model) *OpenAPIGeneratorBuilder {
	b.model = value
	return b
}

// Output sets the output directory.
func (b *OpenAPIGeneratorBuilder) Output(value string) *OpenAPIGeneratorBuilder {
	b.output = value
	return b
}

// Names sets calculator that will be used to calculate names of OpenAPI things.
func (b *OpenAPIGeneratorBuilder) Names(value *openapi.NamesCalculator) *OpenAPIGeneratorBuilder {
	b.names = value
	return b
}

// Binding sets the object that will by used to do HTTP binding calculations.
func (b *OpenAPIGeneratorBuilder) Binding(value *http.BindingCalculator) *OpenAPIGeneratorBuilder {
	b.binding = value
	return b
}

// Packages sets the object that will by used to calculate Go package names.
func (b *OpenAPIGeneratorBuilder) Packages(
	value *golang.PackagesCalculator) *OpenAPIGeneratorBuilder {
	b.packages = value
	return b
}

// Build checks the configuration stored in the builder and, if it is correct, creates a new
// OpenAPI specifications generator using it.
func (b *OpenAPIGeneratorBuilder) Build() (generator *OpenAPIGenerator, err error) {
	// Check that the mandatory parameters have been provided:
	if b.reporter == nil {
		err = fmt.Errorf("reporter is mandatory")
		return
	}
	if b.model == nil {
		err = fmt.Errorf("model is mandatory")
		return
	}
	if b.output == "" {
		err = fmt.Errorf("output directory is mandatory")
		return
	}
	if b.names == nil {
		err = fmt.Errorf("names calculator is mandatory")
		return
	}
	if b.binding == nil {
		err = fmt.Errorf("binding calculator is mandatory")
		return
	}
	if b.packages == nil {
		err = fmt.Errorf("packages calculator is mandatory")
		return
	}

	// Create the generator:
	generator = &OpenAPIGenerator{
		reporter: b.reporter,
		model:    b.model,
		output:   b.output,
		names:    b.names,
		binding:  b.binding,
		packages: b.packages,
	}

	return
}

// Run executes the code generator.
func (g *OpenAPIGenerator) Run() error {
	var err error

	// Generate the OpenAPI specification type for each version:
	for _, service := range g.model.Services() {
		for _, version := range service.Versions() {
			err = g.generateSpec(version)
			if err != nil {
				return err
			}
		}
	}

	// Check if there were errors:
	if g.errors > 0 {
		if g.errors > 1 {
			err = fmt.Errorf("there were %d errors", g.errors)
		} else {
			err = fmt.Errorf("there was 1 error")
		}
		return err
	}

	return nil
}

func (g *OpenAPIGenerator) generateSpec(version *concepts.Version) error {
	var err error

	// Calculate the package name:
	pkgName := g.packages.VersionPackage(version)

	// Create the buffer:
	g.buffer, err = openapi.NewBufferBuilder().
		Reporter(g.reporter).
		Output(g.output).
		Packages(g.packages).
		Package(pkgName).
		Build()
	if err != nil {
		return err
	}

	// Generate the source:
	g.generateSpecSource(version)

	// Write the generated code:
	return g.buffer.Write()
}

func (g *OpenAPIGenerator) generateSpecSource(version *concepts.Version) {
	g.buffer.StartObject()
	g.buffer.Field("openapi", "3.0.0")
	g.generateInfo(version)
	g.generateServers(version)
	g.generatePaths(version)
	g.generateComponents(version)
	g.generateSecurity(version)
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateInfo(version *concepts.Version) {
	g.buffer.StartObject("info")
	g.buffer.Field("version", version.Name().String())
	g.buffer.Field("title", version.Owner().Name().String())
	g.buffer.StartObject("license")
	g.buffer.Field("name", "Apache 2.0")
	g.buffer.Field("url", "http://www.apache.org/licenses/LICENSE-2.0")
	g.buffer.EndObject()
	g.buffer.StartObject("contact")
	g.buffer.Field("name", "OCM Feedback")
	g.buffer.Field("email", "ocm-feedback@redhat.com")
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateServers(version *concepts.Version) {
	g.buffer.StartArray("servers")
	g.buffer.StartObject()
	g.buffer.Field("description", "Production")
	g.buffer.Field("url", "https://api.openshift.com")
	g.buffer.EndObject()
	g.buffer.EndArray()
}

func (g *OpenAPIGenerator) generatePaths(version *concepts.Version) {
	// Calculate the complete URLs for the paths and sort them alphabetically so the order will
	// be predictable:
	index := map[string][]*concepts.Locator{}
	for _, path := range version.Paths() {
		absolute := g.absolutePath(path)
		index[absolute] = path
	}
	absolutes := make([]string, len(index))
	i := 0
	for text := range index {
		absolutes[i] = text
		i++
	}
	sort.Strings(absolutes)

	// Generate the specification:
	g.buffer.StartObject("paths")
	for _, absolute := range absolutes {
		path := index[absolute]
		g.generatePath(path)
	}
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generatePath(path []*concepts.Locator) {
	absolute := g.absolutePath(path)
	resource := path[len(path)-1].Target()
	g.buffer.StartObject(absolute)
	for _, method := range resource.Methods() {
		g.generateMethod(path, method)
	}
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateMethod(path []*concepts.Locator, method *concepts.Method) {
	g.buffer.StartObject(strings.ToLower(g.binding.Method(method)))
	g.generateDescription(method.Doc())
	g.generateParameters(path, method)
	bodyParameters := g.binding.RequestBodyParameters(method)
	if len(bodyParameters) > 0 {
		bodyParameter := bodyParameters[0]
		g.generateRequestBody(bodyParameter)
	}
	g.generateResponses(path, method)
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateParameters(path []*concepts.Locator, method *concepts.Method) {
	g.buffer.StartArray("parameters")
	for _, locator := range path {
		if locator.Variable() {
			g.generatePathParameter(locator)
		}
	}
	for _, parameter := range g.binding.RequestQueryParameters(method) {
		g.generateQueryParameter(parameter)
	}
	g.buffer.EndArray()
}

func (g *OpenAPIGenerator) generatePathParameter(locator *concepts.Locator) {
	g.buffer.StartObject()
	g.buffer.Field("name", g.binding.LocatorSegment(locator)+"_id")
	g.buffer.Field("in", "path")
	g.buffer.StartObject("schema")
	g.buffer.Field("type", "string")
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateQueryParameter(parameter *concepts.Parameter) {
	g.buffer.StartObject()
	g.buffer.Field("name", g.binding.ParameterName(parameter))
	g.generateDescription(parameter.Doc())
	g.buffer.Field("in", "query")
	g.buffer.StartObject("schema")
	g.generateSchemaReference(parameter.Type())
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateRequestBody(parameter *concepts.Parameter) {
	g.buffer.StartObject("requestBody")
	g.buffer.StartObject("content")
	g.buffer.StartObject("application/json")
	g.buffer.StartObject("schema")
	g.generateSchemaReference(parameter.Type())
	g.buffer.EndObject()
	g.buffer.EndObject()
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateResponses(path []*concepts.Locator, method *concepts.Method) {
	g.buffer.StartObject("responses")
	g.buffer.StartObject(g.binding.DefaultStatus(method))
	g.generateDescription("Success.")
	parameters := g.binding.ResponseParameters(method)
	if len(parameters) > 0 {
		g.buffer.StartObject("content")
		g.buffer.StartObject("application/json")
		g.buffer.StartObject("schema")
		if len(parameters) > 1 {
			g.buffer.Field("type", "object")
			g.buffer.StartObject("properties")
			for _, parameter := range parameters {
				g.generateResponseProperty(parameter)
			}
			g.buffer.EndObject()
		} else {
			g.generateSchemaReference(parameters[0].Type())
		}
		g.buffer.EndObject()
		g.buffer.EndObject()
		g.buffer.EndObject()
	}
	g.buffer.EndObject()
	g.buffer.StartObject("default")
	g.generateDescription("Error.")
	g.buffer.StartObject("content")
	g.buffer.StartObject("application/json")
	g.buffer.StartObject("schema")
	g.buffer.Field("$ref", "#/components/schemas/Error")
	g.buffer.EndObject()
	g.buffer.EndObject()
	g.buffer.EndObject()
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateResponseProperty(parameter *concepts.Parameter) {
	name := g.names.ParameterPropertyName(parameter)
	g.buffer.StartObject(name)
	g.generateDescription(parameter.Doc())
	g.generateSchemaReference(parameter.Type())
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateComponents(version *concepts.Version) {
	g.buffer.StartObject("components")
	g.buffer.StartObject("schemas")
	for _, typ := range version.Types() {
		g.generateSchema(typ)
	}
	g.generateErrorSchema()
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateSchema(typ *concepts.Type) {
	switch {
	case typ.IsEnum():
		g.generateEnumSchema(typ)
	case typ.IsStruct():
		g.generateStructSchema(typ)
	}
}

func (g *OpenAPIGenerator) generateEnumSchema(typ *concepts.Type) {
	name := g.names.SchemaName(typ)
	g.buffer.StartObject(name)
	g.generateDescription(typ.Doc())
	g.buffer.Field("type", "string")
	g.buffer.StartArray("enum")
	for _, value := range typ.Values() {
		g.buffer.Item(g.binding.EnumValueName(value))
	}
	g.buffer.EndArray()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateStructSchema(typ *concepts.Type) {
	name := g.names.SchemaName(typ)
	g.buffer.StartObject(name)
	g.generateDescription(typ.Doc())
	g.buffer.StartObject("properties")
	if typ.IsClass() {
		// Kind:
		g.buffer.StartObject("kind")
		g.generateDescription(fmt.Sprintf(
			"Indicates the type of this object. Will be '%s' if this is a complete "+
				"object or '%sLink' if it is just a link.",
			name, name,
		))
		g.buffer.Field("type", "string")
		g.buffer.EndObject()

		// ID:
		g.buffer.StartObject("id")
		g.generateDescription("Unique identifier of the object.")
		g.buffer.Field("type", "string")
		g.buffer.EndObject()

		// HREF:
		g.buffer.StartObject("href")
		g.generateDescription("Self link.")
		g.buffer.Field("type", "string")
		g.buffer.EndObject()
	}
	for _, attribute := range typ.Attributes() {
		g.generateStructProperty(attribute)
	}
	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateStructProperty(attribute *concepts.Attribute) {
	name := g.names.AttributePropertyName(attribute)
	g.buffer.StartObject(name)
	g.generateDescription(attribute.Doc())
	g.generateSchemaReference(attribute.Type())
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateSecurity(version *concepts.Version) {
	g.buffer.StartArray("security")
	g.buffer.StartObject()
	g.buffer.StartArray("bearer")
	g.buffer.EndArray()
	g.buffer.EndObject()
	g.buffer.EndArray()
}

func (g *OpenAPIGenerator) generateSchemaReference(typ *concepts.Type) {
	version := typ.Owner()
	switch {
	case typ == version.Boolean():
		g.buffer.Field("type", "boolean")
	case typ == version.Integer():
		g.buffer.Field("type", "integer")
		g.buffer.Field("format", "int32")
	case typ == version.Long():
		g.buffer.Field("type", "integer")
		g.buffer.Field("format", "int64")
	case typ == version.Float():
		g.buffer.Field("type", "number")
		g.buffer.Field("format", "float")
	case typ == version.String():
		g.buffer.Field("type", "string")
	case typ == version.Date():
		g.buffer.Field("type", "string")
		g.buffer.Field("format", "date-time")
	case typ.IsEnum() || typ.IsStruct():
		g.buffer.Field("$ref", "#/components/schemas/"+g.names.SchemaName(typ))
	case typ.IsList():
		g.buffer.Field("type", "array")
		g.buffer.StartObject("items")
		g.generateSchemaReference(typ.Element())
		g.buffer.EndObject()
	case typ.IsMap():
		g.buffer.Field("type", "object")
		g.buffer.StartObject("additionalProperties")
		g.generateSchemaReference(typ.Element())
		g.buffer.EndObject()
	default:
		g.reporter.Errorf(
			"Don't know how to generate schema reference for type '%s'",
			typ.Name(),
		)
	}
}

func (g *OpenAPIGenerator) generateErrorSchema() {
	g.buffer.StartObject("Error")
	g.buffer.Field("type", "object")
	g.buffer.StartObject("properties")

	// Kind:
	g.buffer.StartObject("kind")
	g.generateDescription("Indicates the type of this object. Will always be 'Error'")
	g.buffer.Field("type", "string")
	g.buffer.EndObject()

	// ID:
	g.buffer.StartObject("id")
	g.generateDescription("Numeric identifier of the error.")
	g.buffer.Field("type", "integer")
	g.buffer.Field("format", "int32")
	g.buffer.EndObject()

	// HREF:
	g.buffer.StartObject("href")
	g.generateDescription("Self link.")
	g.buffer.Field("type", "string")
	g.buffer.EndObject()

	// Code:
	g.buffer.StartObject("code")
	g.generateDescription(
		"Globally unique code of the error, composed of the unique identifier of the API " +
			"and the numeric identifier of the error. For example, for if the " +
			"numeric identifier of the error is `93` and the identifier of the API " +
			"is `clusters_mgmt` then the code will be `CLUSTERS-MGMT-93`.",
	)
	g.buffer.Field("type", "string")
	g.buffer.EndObject()

	// Reason:
	g.buffer.StartObject("reason")
	g.generateDescription("Human readable description of the error.")
	g.buffer.Field("type", "string")
	g.buffer.EndObject()

	// Details:
	g.buffer.StartObject("details")
	g.generateDescription("Extra information about the error.")
	g.buffer.Field("type", "object")
	g.buffer.Field("additionalProperties", "true")
	g.buffer.EndObject()

	g.buffer.EndObject()
	g.buffer.EndObject()
}

func (g *OpenAPIGenerator) generateDescription(doc string) {
	if doc != "" {
		g.buffer.Field("description", doc)
	}
}

func (g *OpenAPIGenerator) absolutePath(path []*concepts.Locator) string {
	version := path[0].Owner().Owner()
	service := version.Owner()
	segments := make([]string, len(path))
	for i, locator := range path {
		if locator.Variable() {
			segments[i] = fmt.Sprintf("{%s_id}", locator.Name().String())
		} else {
			segments[i] = locator.Name().String()
		}
	}
	return fmt.Sprintf(
		"/api/%s/%s/%s",
		service.Name(),
		version.Name(),
		strings.Join(segments, "/"),
	)
}