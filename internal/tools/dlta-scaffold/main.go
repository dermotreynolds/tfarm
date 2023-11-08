// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	gomonkey "github.com/agiledragon/gomonkey/v2"
	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	help "github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/provider"
	"github.com/hashicorp/terraform-provider-azurerm/internal/sdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
)

// NOTE: since we're using `go run` for these tools all of the code needs to live within the main.go

type documentationGenerator struct {
	resource *schema.Resource

	// resourceName is the name of the resource e.g. `azurerm_resource_group`
	resourceName string

	// isDataSource defines if this is a Data Source (if not it's a Resource)
	isDataSource bool

	// dltaPath gives the path to the dlta configuration
	dltaPath string

	//TODO may not be needed if using is DataSource
	isResource bool

	isForced bool

	ShortCode string

	NamingConvention string
}

type NameValue map[string]interface{}

type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PaletteProp struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Description  *string     `json:"description"`
	Type         string      `json:"type"`
	CurrentValue interface{} `json:"value"`
	FlattenName  *string     `json:"flatten_name"`
	Filter       *string     `json:"filter"`
	Disabled     bool        `json:"disabled"`
	ReadOnly     bool        `json:"readonly"`
	Validators   NameValue   `json:"validators"`
	Options      []KeyValue  `json:"options"`
}

type PaletteObj struct {
	SVG   string `json:"svg"`
	Color string `json:"color"`
}

type Creator struct {
	CreateFunction string `json:"create_function"`

	Props []PaletteProp `json:"controls"`
}

type namingStruct struct {
	Delimiter    string
	StaticName   string
	IsDataSource bool
	Prefix       string
	Fields       []string
}

type attribute struct {
	Description     string
	IsBlock         bool
	MaxItems        int
	MinItems        int
	Required        bool
	Optional        bool
	Computed        bool
	ForceNew        bool
	PossibleValues  []string
	PossibleOptions []string
	DataTypeString  string
	Attributes      map[string]attribute
	Default         string //TODO Find out how this works  SchemaDefaultFunc
	ConflictsWith   []string
	ResourcePath    string
}

// a.IsBlock = isBlock
// a.DataTypeString = s.Type.String()
// a.ResourcePath = parentPath + "." + fieldName

type attributeSummary struct {
	IsBlock        bool
	DataTypeString string
	ResourcePath   string
	Attributes     map[string]attributeSummary
	Published      bool
}

type summaryAttribute struct {
	Published             bool
	IsBlock               bool
	Required              bool
	Optional              bool
	Computed              bool
	DependentResourcePath string
}

// Variables
var (
	terraform_azurerm_azurerm_source = attribute{
		DataTypeString: "TypeString",
		Description:    "String representing the source",
	}
	terraform_azurerm_azurerm_version = attribute{
		DataTypeString: "TypeString",
		Description:    "String representing the source",
	}

	terraform_azurerm_azapi_source = attribute{
		DataTypeString: "TypeString",
		Description:    "String representing the source",
	}
	terraform_azurerm_azapi_version = attribute{
		DataTypeString: "TypeString",
		Description:    "String representing the version",
	}

	dlta_terraform_template = attribute{
		DataTypeString: "TypeString",
		Description:    "Terraform template to provision this asset",
	}
	dlta_naming_convention = attribute{
		DataTypeString: "TypeString",
		Description:    "String that defines the naming convention for this asset type",
	}

	dlta_environment_char = attribute{
		DataTypeString: "TypeString",
		Description:    "Single character variable",
	}

	dlta_application_short_code = attribute{
		DataTypeString: "TypeString",
		Description:    "Short code representing the application",
	}
	dlta_business_short_code = attribute{
		DataTypeString: "TypeString",
		Description:    "Short code representing the business",
	}
	dlta_instance_id = attribute{
		DataTypeString: "TypeString",
		Description:    "3 digit string representing the instance",
	}
	dlta_location_short_code = attribute{
		DataTypeString: "TypeString",
		Description:    "3 digit string representing the region",
	}

	dlta_vendor_asset_short_code = attribute{
		DataTypeString: "TypeString",
		Description:    "short string representing the asset type",
	}

	dlta_terraform_module_name = attribute{
		DataTypeString: "TypeString",
		Description:    "Terraform Module Name e.g. azurerm_service_plan_1234",
	}

	dlta_terraform_data_source_name = attribute{
		DataTypeString: "TypeString",
		Description:    "Terraform Data Source Name e.g. ds-kvap-sec-demo-d-eun-001",
	}

	dlta_terraform_is_data_source = attribute{
		DataTypeString: "TypeString",
		Description:    "Terraform Data Source Tyoe e.g. resource|data",
	}

	dlta_location_options = []KeyValue{
		{Key: "northeurope", Value: "northeurope"},
		{Key: "westeurope", Value: "westeurope"},
	}

	dlta_environment_char_options = []KeyValue{
		{Key: "d", Value: "d"},
		{Key: "u", Value: "u"},
		{Key: "s", Value: "s"},
		{Key: "p", Value: "p"},
	}

	dlta_application_short_code_options = []KeyValue{
		{Key: "demo", Value: "demo"},
		{Key: "bigd", Value: "bigd"},
		{Key: "ecom", Value: "ecom"},
		{Key: "erp", Value: "erp"},
		{Key: "aldft", Value: "aldft"},
	}

	dlta_business_short_code_options = []KeyValue{
		{Key: "sec", Value: "sec"},
		{Key: "idt", Value: "idt"},
		{Key: "mgt", Value: "mgt"},
		{Key: "finc", Value: "finc"},
		{Key: "gdmz", Value: "gdmz"},
	}

	dlta_instance_id_options = []KeyValue{
		{Key: "001", Value: "001"},
		{Key: "002", Value: "002"},
		{Key: "003", Value: "003"},
	}

	dlta_location_short_code_options = []KeyValue{
		{Key: "eun", Value: "eun"},
		{Key: "euw", Value: "euw"},
	}

	terraform_azurerm_azurerm_source_options = []KeyValue{
		{Key: "hashicorp/azurerm", Value: "hashicorp/azurerm"},
	}

	terraform_azurerm_azurerm_version_options = []KeyValue{
		{Key: "3.59.0", Value: "3.59.0"},
	}

	terraform_azurerm_azapi_source_options = []KeyValue{
		{Key: "azure/azapi", Value: "azure/azapi"},
	}

	terraform_azurerm_azapi_version_options = []KeyValue{
		{Key: "1.6.0", Value: "1.6.0"},
	}
)

type Artefact int64

const (
	PublishedPropertiesSummary Artefact = iota
	TerraformTemplate
	ModuleBlock
	VariableBlock
	LocalBlock
	OutputBlock
	PalletteBlock
)

func main() {
	f := flag.NewFlagSet("example", flag.ExitOnError)

	resourceName := f.String("name", "", "The name of the Data Source/Resource which should be generated")
	resourceType := f.String("type", "", "Whether this is a Data Source (data) or a Resource (resource)")
	dltaPath := f.String("dlta-path", "", "The relative path to the dlta folder")
	outputType := f.String("output-type", "", "Custom prop")

	force := f.String("force", "n", "Custom prop")

	_ = f.Parse(os.Args[1:])

	quitWithError := func(message string) {
		log.Print(message)
		os.Exit(1)
	}

	if resourceName == nil || *resourceName == "" {
		quitWithError("The name of the Data Source/Resource must be specified via `-name`")
		return
	}

	if resourceType == nil || *resourceType == "" {
		quitWithError("The type of the Data Source/Resource must be specified via `-type`")
		return
	}

	if *resourceType != "data" && *resourceType != "resource" {
		quitWithError("The type of the Data Source/Resource specified via `-type` must be either `data` or `resource`")
		return
	}

	if *outputType != "init" && *outputType != "scaffold" && *outputType != "config" {
		quitWithError("`-output-type` must be either `init`, `scaffold` or `config`")
		return
	}

	if dltaPath == nil || *dltaPath == "" {
		quitWithError("The Relative Website Path must be specified via `-dlta-path`")
		return
	}

	if force == nil || *force == "" {
		quitWithError("The Relative Website Path must be specified via `-dlta-path`")
		return
	}

	isForced := *force == "y"
	isResource := *resourceType == "resource"

	if err := run(*resourceName, isResource, *dltaPath, *outputType, isForced); err != nil {
		panic(err)
	}
}

func run(resourceName string, isResource bool, dltaPath string, outputType string, isForced bool) error {
	_, err := getContent(resourceName, isResource, dltaPath, outputType, isForced)
	if err != nil {
		return fmt.Errorf("building content: %s", err)
	}

	return err
	// return saveContent(resourceName, websitePath, *content, isResource)
}

func getContent(resourceName string, isResource bool, dltaPath string, outputType string, isForced bool) (*string, error) {
	generator := documentationGenerator{
		resourceName: resourceName,
		isDataSource: !isResource,
		// exampleSource: expsrc,
		dltaPath:   dltaPath,
		isResource: isResource,
		isForced:   isForced,
	}

	if resourceName != "terraform_azurerm" && resourceName != "devops_pipeline" {

		if !isResource {
			for _, service := range provider.SupportedTypedServices() {
				for _, ds := range service.DataSources() {
					if ds.ResourceType() == resourceName {
						wrapper := sdk.NewDataSourceWrapper(ds)
						dsWrapper, err := wrapper.DataSource()
						if err != nil {
							return nil, fmt.Errorf("wrapping Data Source %q: %+v", ds.ResourceType(), err)
						}

						generator.resource = dsWrapper
						// generator.websiteCategories = service.WebsiteCategories()
						break
					}
				}
			}
			for _, service := range provider.SupportedUntypedServices() {
				for key, ds := range service.SupportedDataSources() {
					if key == resourceName {
						generator.resource = ds
						// generator.websiteCategories = service.WebsiteCategories()
						break
					}
				}
			}

			if generator.resource == nil {
				return nil, fmt.Errorf("Data Source %q was not registered!", resourceName)
			}
		} else {
			for _, service := range provider.SupportedTypedServices() {
				for _, rs := range service.Resources() {
					if rs.ResourceType() == resourceName {
						wrapper := sdk.NewResourceWrapper(rs)
						rsWrapper, err := wrapper.Resource()
						if err != nil {
							return nil, fmt.Errorf("wrapping Resource %q: %+v", rs.ResourceType(), err)
						}

						generator.resource = rsWrapper
						// generator.websiteCategories = service.WebsiteCategories()
						break
					}
				}
			}
			for _, service := range provider.SupportedUntypedServices() {
				for key, rs := range service.SupportedResources() {
					if key == resourceName {
						generator.resource = rs
						// generator.websiteCategories = service.WebsiteCategories()
						break
					}
				}
			}

			if generator.resource == nil {
				return nil, fmt.Errorf("Resource %q was not registered!", resourceName)
			}
		}
	} else {
		generator.resourceName = resourceName
	}

	generator.ShortCode = getResourceShortCode(generator.resourceName)
	generator.NamingConvention = generator.getResourceNamingConvention(generator.resourceName, generator.isDataSource)

	if outputType == "init" {
		_ = generator.writeInitResourceProperties()
		// _ = generator.writeAllInputAttributesSummary()
	} else if outputType == "scaffold" {
		_ = generator.scaffoldConfiguation()
		// return &docs, nil
	}

	return nil, nil
}

// Full Attributes
func (gen documentationGenerator) getAllInputAttributes(input map[string]*schema.Schema, parent attribute, isChild bool, parentPath string) map[string]attribute {

	// resourceName := gen.resourceName
	// gen.resource.Schema, gen.resourceName, attribute{}, false, gen.resourceName
	retAttributes := make(map[string]attribute)

	for _, fieldName := range gen.sortFields(input) {

		if input[fieldName].Required || input[fieldName].Optional {
			a := attribute{}
			if isBlock(input[fieldName]) {

				cloneSchemaToAttributes(&a, input[fieldName], true, parentPath, fieldName)
				//attrib = attribute{IsBlock: true, MaxItems: input[fieldName].MaxItems, Required: input[fieldName].Required, DataTypeString: input[fieldName].Type.String(), Optional: input[fieldName].Optional, MinItems: input[fieldName].MinItems, ForceNew: input[fieldName].ForceNew}
				// retap := gen.getAllAttributes(input[fieldName].Elem.(*schema.Resource).Schema, resourceName, a, true, parentPath+"."+fieldName)

				retap := gen.getAllInputAttributes(input[fieldName].Elem.(*schema.Resource).Schema, a, true, parentPath+"."+fieldName)
				a.Attributes = retap
			} else {
				cloneSchemaToAttributes(&a, input[fieldName], false, parentPath, fieldName)

				//attrib = attribute{IsBlock: false, MaxItems: input[fieldName].MaxItems, Required: input[fieldName].Required, DataTypeString: input[fieldName].Type.String(), Optional: input[fieldName].Optional, MinItems: input[fieldName].MinItems, ForceNew: input[fieldName].ForceNew}

				b := input[fieldName]

				if possibleValues := getSchemaPossibleValues(b); len(possibleValues) > 0 {
					for i := 0; i < len(possibleValues); i++ {
						a.PossibleValues = append(a.PossibleValues, possibleValues[i])

					}
				}

				b, isSchema := input[fieldName].Elem.(*schema.Schema)

				if isSchema {
					if possibleValues := getSchemaPossibleValues(b); len(possibleValues) > 0 {
						for i := 0; i < len(possibleValues); i++ {
							a.PossibleValues = append(a.PossibleValues, possibleValues[i])

						}
					}
				}
				// writeDebugJson(attrib)
				// fmt.Printf("field	%s	%t\n", fieldName, isChild)
			}

			retAttributes[fieldName] = a
		}

	}

	return retAttributes

}

func (gen documentationGenerator) getAllOutputAttributes(input map[string]*schema.Schema, parent attribute, isChild bool, parentPath string) map[string]attribute {

	retAttributes := make(map[string]attribute)

	var (
		id = attribute{
			DataTypeString: "TypeString",
			Description:    "The resource id",
		}

		name = attribute{
			DataTypeString: "TypeString",
			Description:    "The resource name",
		}
	)

	retAttributes["id"] = id
	retAttributes["name"] = name

	// writeDebugJson(retAttributes)
	return retAttributes
}

func (gen documentationGenerator) summariseAttributes(a map[string]attribute, rn string, parentRequired bool) map[string]summaryAttribute {

	printAttributes := func(a map[string]attribute, rn string) map[string]summaryAttribute {
		retAttributes := make(map[string]summaryAttribute)
		for k, a := range a {
			published := false
			if parentRequired {
				if a.Required {
					published = true
				}
			} else {
				published = false
			}
			newrn := rn + "." + k

			if a.IsBlock {
				retAttributes[a.ResourcePath] = summaryAttribute{Published: published, IsBlock: a.IsBlock, Required: a.Required, Optional: a.Optional, Computed: a.Computed}
				a := gen.summariseAttributes(a.Attributes, newrn, published)
				for n, v := range a {
					retAttributes[n] = summaryAttribute{Published: v.Published, IsBlock: v.IsBlock}
				}

			} else {
				retAttributes[a.ResourcePath] = summaryAttribute{Published: published, IsBlock: a.IsBlock, Required: a.Required, Optional: a.Optional, Computed: a.Computed}
			}
		}

		return retAttributes
	}

	return printAttributes(a, rn)
}

func (gen documentationGenerator) writeResource(s string, a Artefact) string {

	var fileName string
	var subDir string

	resourceKind := "r"
	if !gen.isResource {
		resourceKind = "d"
	}

	// fileName := strings.TrimPrefix(gen.resourceName, "azurerm_")
	if a == TerraformTemplate {
		fileName = "template.json"
		subDir = "resource"
	} else if a == ModuleBlock {
		fileName = "main.tf"
		subDir = "module"
	} else if a == VariableBlock {
		fileName = "variables.tf"
		subDir = "module"
	} else if a == PalletteBlock {
		fileName = "pallette.sql"
		subDir = "resource"
	} else if a == OutputBlock {
		fileName = "output.tf"
		subDir = "module"
	} else if a == LocalBlock {
		fileName = "local.tf"
		subDir = "module"
	}

	dirName := gen.resourceName
	// /home/dermot/source/repo/Repo.DltaModules

	outputDirectoryName := fmt.Sprintf("/%s/%s/%s/%s/", gen.dltaPath, resourceKind, dirName, subDir)
	// outputDirectoryPath, err := filepath.Abs(outputDirectoryName)
	outputDirectoryPath := outputDirectoryName

	// if err != nil {
	// 	fmt.Printf("writeResource \"0. directory error\": %v\n", err.Error())
	// }

	outputFileName := fmt.Sprintf("/%s/%s/%s/%s/%s", gen.dltaPath, resourceKind, dirName, subDir, fileName)
	// outputPath, err := filepath.Abs(outputFileName)

	outputPath := outputFileName

	// if err != nil {
	// 	fmt.Printf("writeResource \"1. file error\": %v\n", err.Error())
	// }

	if gen.isForced {
		if _, err := os.Stat(outputPath); err == nil {
			// fmt.Printf("initResourceProperties \"2. Clearing out previous version of file\": %s\n", outputPath)
			os.Remove(outputPath)
		}
	}

	if _, err := os.Stat(outputPath); err == nil {

		fmt.Printf("writeResource \"3. File exists error\"  on path: %s\n", outputPath)

		// fmt.Printf("\"After stat, file exists error\": %s\n", outputPath)
	} else {
		err := os.MkdirAll(outputDirectoryPath, os.ModePerm)
		if err != nil {
			fmt.Printf("writeResource \"4.1 directory error\": %v\n", err.Error())
		}

		file, err := os.Create(outputPath)
		if err != nil {
			fmt.Printf("writeResource \"4. file error\": %v\n", err.Error())
		}
		if os.IsExist(err) {
			os.Remove(outputPath)
			file, err = os.Create(outputPath)
			if err != nil {
				fmt.Printf("writeResource \"5. file error\": %v\n", err.Error())
			}
		}
		defer file.Close()

		// s = strings.TrimSpace(s)
		_, _ = file.WriteString(s)
		file.Sync()
	}

	return ""
}

func (gen documentationGenerator) writeInitResourceProperties() string {

	if gen.resourceName != "terraform_azurerm" && gen.resourceName != "devops_pipeline" {
		attributes := gen.getAllInputAttributes(gen.resource.Schema, attribute{}, false, gen.resourceName)

		// f := gen.getAllInputAttributesSummary(gen.resource.Schema, attributeSummary{}, false, gen.resourceName)
		// writeDebugJson(f)

		flatted := gen.summariseAttributes(attributes, gen.resourceName, true)

		content := writeJson(flatted)

		resourceKind := "r"
		if !gen.isResource {
			resourceKind = "d"
		}

		// fileName := strings.TrimPrefix(gen.resourceName, "azurerm_")
		fileName := gen.resourceName
		dirName := gen.resourceName
		subDir := "resource"
		outputDirectoryName := fmt.Sprintf("/%s/%s/%s/%s/", gen.dltaPath, resourceKind, dirName, subDir)
		// outputDirectoryPath, err := filepath.Abs(outputDirectoryName)

		outputDirectoryPath := outputDirectoryName

		// if err != nil {
		// 	fmt.Printf("initResourceProperties \"0. directory error\": %v\n", err.Error())
		// }

		outputFileName := fmt.Sprintf("%s/%s/%s/%s/%s.json", gen.dltaPath, resourceKind, dirName, subDir, fileName)
		// outputPath, err := filepath.Abs(outputFileName)

		outputPath := outputFileName

		// if err != nil {
		// 	fmt.Printf("initResourceProperties \"1. file error\": %v\n", err.Error())
		// }

		if gen.isForced {
			if _, err := os.Stat(outputPath); err == nil {
				// fmt.Printf("initResourceProperties \"2. Clearing out previous version of file\": %s\n", outputPath)
				os.Remove(outputPath)
			}
		}

		if _, err := os.Stat(outputPath); err == nil {

			fmt.Printf("initResourceProperties \"3. File exists error\": %s\n", outputPath)

			// fmt.Printf("\"After stat, file exists error\": %s\n", outputPath)
		} else {
			err := os.MkdirAll(outputDirectoryPath, os.ModePerm)
			if err != nil {
				fmt.Printf("initResourceProperties \"4.1 directory error\": %v\n", err.Error())
			}

			file, err := os.Create(outputPath)
			if err != nil {
				fmt.Printf("initResourceProperties \"4. file error\": %v\n", err.Error())
			}
			if os.IsExist(err) {
				os.Remove(outputPath)
				file, err = os.Create(outputPath)
				if err != nil {
					fmt.Printf("initResourceProperties \"5. file error\": %v\n", err.Error())
				}
			}
			defer file.Close()

			content = strings.TrimSpace(content)
			_, _ = file.WriteString(content)
			file.Sync()
		}
	}
	return ""
}

func (gen documentationGenerator) readResourceProperties() map[string]summaryAttribute {

	data := make(map[string]summaryAttribute)

	if gen.resourceName != "terraform_azurerm" && gen.resourceName != "devops_pipeline" {

		resourceKind := "r"
		if !gen.isResource {
			resourceKind = "d"
		}

		fileName := gen.resourceName
		dirName := gen.resourceName
		subDir := "resource"

		outputFileName := fmt.Sprintf("%s/%s/%s/%s/%s.json", gen.dltaPath, resourceKind, dirName, subDir, fileName)

		outputPath, err := filepath.Abs(outputFileName)
		if err != nil {
			fmt.Printf("readResourceProperties \"1. file error\": %v\n", err.Error())
			return data
		}

		if _, err := os.Stat(outputPath); err != nil {
			fmt.Printf("readResourceProperties \"2. File does not exist\": %s\n", outputPath)
			fmt.Printf("readResourceProperties \"2. You may not have run init to create\": %s\n", outputPath)
			return data
		}

		fileContent, err := os.ReadFile(outputPath)

		if err != nil {
			fmt.Printf("readResourceProperties \"file error\": %v\n", err.Error())
			return data
		}

		_ = json.Unmarshal([]byte(fileContent), &data)
	}
	return data
}

func (gen documentationGenerator) getPublishedAttributes() map[string]attribute {

	publishedAttributes := make(map[string]attribute)

	if gen.resource != nil {
		inputAttributes := gen.getAllInputAttributes(gen.resource.Schema, attribute{}, false, gen.resourceName)

		sa := gen.readResourceProperties()

		// sa := summariseAttributesForConfig(attributes, gen.resourceName, true)

		publishedAttributes = gen.getAllPublishedAttributes(inputAttributes, sa)
	}

	return publishedAttributes
}

func (gen documentationGenerator) getAllPublishedAttributes(allAttr map[string]attribute, sa map[string]summaryAttribute) map[string]attribute {

	retAttributes := make(map[string]attribute)

	for k, a := range allAttr {

		t := attribute{}

		if !sa[a.ResourcePath].Published {
			continue
		}
		t = a

		if a.IsBlock {
			tempAttributes := make(map[string]attribute)
			for k2, a2 := range gen.getAllPublishedAttributes(a.Attributes, sa) {
				if sa[a2.ResourcePath].Published {
					tempAttributes[k2] = a2
				}
			}
			t.Attributes = tempAttributes

		}

		retAttributes[k] = t
	}

	return retAttributes
}

func (gen documentationGenerator) scaffoldConfiguation() string {

	gen.writeResource(gen.terraformTemplateBlock(), TerraformTemplate)

	// writeDebug("#### Template block:\n" + gen.terraformTemplateBlock() + "\n")
	gen.writeResource(gen.terraformModuleBlock(), ModuleBlock)
	// writeDebug("#### Module block:\n" + gen.terraformModuleBlock() + "\n")

	gen.writeResource(gen.terraformVariableBlock(), VariableBlock)
	// writeDebug("#### Variable block:\n" + gen.terraformVariableBlock() + "\n")

	gen.writeResource(gen.terraformLocalBlock(), LocalBlock)
	// writeDebug("#### Local block:\n" + gen.terraformLocalBlock() + "\n")

	gen.writeResource(gen.dltaPalletteCodeBlock(), PalletteBlock)
	// writeDebug("#### Pallette block:\n" + gen.dltaPalletteCodeBlock() + "\n")

	gen.writeResource(gen.terraformOutputBlock(), OutputBlock)
	// writeDebug("#### Output block:\n" + gen.terraformOutputBlock() + "\n")

	//TODO
	// OutputBlock

	return ""
}

func (gen documentationGenerator) getInjectAttributes() map[string]attribute {

	injectAttributes := make(map[string]attribute)

	if gen.resourceName == "terraform_azurerm" {
		injectAttributes["terraform_azurerm_azurerm_source"] = terraform_azurerm_azurerm_source
		injectAttributes["terraform_azurerm_azurerm_version"] = terraform_azurerm_azurerm_version
		injectAttributes["terraform_azurerm_azapi_source"] = terraform_azurerm_azapi_source
		injectAttributes["terraform_azurerm_azapi_version"] = terraform_azurerm_azapi_version
		injectAttributes["dlta_terraform_template"] = dlta_terraform_template
		injectAttributes["dlta_naming_convention"] = dlta_naming_convention

	} else if gen.resourceName == "devops_pipeline" {

		//TODO
	} else {

		//TODO Dedupe this
		if gen.isDataSource {
			injectAttributes["dlta_environment_char"] = dlta_environment_char
			injectAttributes["dlta_application_short_code"] = dlta_application_short_code
			injectAttributes["dlta_business_short_code"] = dlta_business_short_code
			injectAttributes["dlta_instance_id"] = dlta_instance_id
			injectAttributes["dlta_location_short_code"] = dlta_location_short_code
			injectAttributes["dlta_vendor_asset_short_code"] = dlta_vendor_asset_short_code
			injectAttributes["dlta_terraform_template"] = dlta_terraform_template
			injectAttributes["dlta_naming_convention"] = dlta_naming_convention
			injectAttributes["dlta_terraform_module_name"] = dlta_terraform_module_name
			injectAttributes["dlta_terraform_data_source_name"] = dlta_terraform_data_source_name
			injectAttributes["dlta_terraform_is_data_source"] = dlta_terraform_is_data_source
		} else {
			injectAttributes["dlta_environment_char"] = dlta_environment_char
			injectAttributes["dlta_application_short_code"] = dlta_application_short_code
			injectAttributes["dlta_business_short_code"] = dlta_business_short_code
			injectAttributes["dlta_instance_id"] = dlta_instance_id
			injectAttributes["dlta_location_short_code"] = dlta_location_short_code
			injectAttributes["dlta_vendor_asset_short_code"] = dlta_vendor_asset_short_code
			injectAttributes["dlta_terraform_template"] = dlta_terraform_template
			injectAttributes["dlta_naming_convention"] = dlta_naming_convention
			injectAttributes["dlta_terraform_module_name"] = dlta_terraform_module_name
			injectAttributes["dlta_terraform_is_data_source"] = dlta_terraform_is_data_source
		}

	}

	return injectAttributes
}

func (gen documentationGenerator) injectAttributes() map[string]attribute {

	allAttributes := make(map[string]attribute)

	injectAttributes := gen.getInjectAttributes()
	publishedAttributes := gen.getPublishedAttributes()

	// allAttributes = injectAttributes + publishedAttributes

	for k, a := range injectAttributes {
		allAttributes[k] = a
	}

	for k, a := range publishedAttributes {
		allAttributes[k] = a
	}

	return allAttributes
}

func (gen documentationGenerator) terraformTemplateBlock() string {

	attributes := gen.injectAttributes()

	var templateBlock string
	if gen.resourceName != "terraform_azurerm" && gen.resourceName != "devops_pipeline" {
		//TODO  Add module name

		if gen.isDataSource {
			//data "azurerm_subnet" "sub" {

			templateBlock += fmt.Sprintf("data \"%s\" \"${%s}\" {\n", gen.resourceName, "dlta_terraform_module_name")

			// templateBlock += fmt.Sprintf("\tsource                      = \"__repo_path__/%s//module?ref=main\"\n", "Module."+resource.Name)
		} else {
			templateBlock += fmt.Sprintf("module \"${%s}\" {\n", "dlta_terraform_module_name")

			templateBlock += fmt.Sprintf("\tsource                      = \"__modules_path__//r//%s//module?ref=main\"\n", gen.resourceName)
		}

		if attributes["location"].DataTypeString != "" {
			templateBlock += fmt.Sprintf("\tlocation                    = \"${%s}\"\n", "location")
		}

		templateBlock += fmt.Sprintf("\tdlta_location_short_code    = ${%s}\n", "dlta_location_short_code")
		templateBlock += fmt.Sprintf("\tdlta_environment_char       = ${%s}\n", "dlta_environment_char")
		templateBlock += fmt.Sprintf("\tdlta_business_short_code    = ${%s}\n", "dlta_business_short_code")
		templateBlock += fmt.Sprintf("\tdlta_application_short_code = ${%s}\n", "dlta_application_short_code")
		templateBlock += fmt.Sprintf("\tdlta_instance_id            = ${%s}\n", "dlta_instance_id")
		templateBlock += fmt.Sprintf("\tdlta_vendor_asset_short_code	= ${%s}\n", "dlta_vendor_asset_short_code")

		for n, at := range attributes {
			// Exclude location as we are overriding the name above
			// Exclude name as this will be
			if n == "location" || strings.Contains(n, "dlta") || n == "name" {
				continue
			} else {
				if !at.Computed {

					if !at.IsBlock {

						if n == "resource_group_name" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tresource_group_name		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tresource_group_name		= module.${ResourceGroup}.name\n") // BUG, Resource Group is camel case in solution
							}

						} else if n == "virtual_network_name" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tvirtual_network_name		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tvirtual_network_name		= module.${virtual_network_name}.name\n") // BUG, Resource Group is camel case in solution
							}
						} else if n == "private_connection_resource_id" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tprivate_connection_resource_id		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tprivate_connection_resource_id		= module.${private_connection_resource_id}.id\n") // BUG, Resource Group is camel case in solution
							}
						} else if n == "subnet_id" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tsubnet_id		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tsubnet_id		= module.${subnet_id}.id\n") // BUG, Resource Group is camel case in solution
							}
						} else if n == "service_plan_id" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tservice_plan_id		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tservice_plan_id		= module.${service_plan_id}.id\n") // BUG, Resource Group is camel case in solution
							}
						} else if n == "storage_account_name" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tstorage_account_name		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tstorage_account_name		= module.${storage_account_name}.name\n") // BUG, Resource Group is camel case in solution
							}
						} else if n == "storage_uses_managed_identity" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tstorage_account_name		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tstorage_uses_managed_identity				= ${storage_uses_managed_identity}\n") // BUG, Resource Group is camel case in solution
							}
						} else if n == "virtual_network_subnet_id" {
							if gen.isDataSource {
								templateBlock += fmt.Sprintf("\tstorage_account_name		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
							} else {
								templateBlock += fmt.Sprintf("\tvirtual_network_subnet_id				= module.${virtual_network_subnet_id}.id\n") // BUG, Resource Group is camel case in solution
							}
						} else {
							if at.DataTypeString == schema.TypeList.String() {
								templateBlock += fmt.Sprintf("\t%s		= ${%s}\n", n, n)
							} else {
								templateBlock += fmt.Sprintf("\t%s		= \"${%s}\"\n", n, n)
							}

						}
					} else {

						for n1, at1 := range at.Attributes {
							if !at1.IsBlock {
								if n1 == "name" {
									continue
								} else if n1 == "private_connection_resource_id" {
									if gen.isDataSource {
										templateBlock += fmt.Sprintf("\tprivate_connection_resource_id		= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
									} else {
										templateBlock += fmt.Sprintf("\tprivate_connection_resource_id		= module.${private_connection_resource_id}.id\n") // BUG, Resource Group is camel case in solution
									}
								} else if n1 == "is_manual_connection		" {
									if gen.isDataSource {
										templateBlock += fmt.Sprintf("\tis_manual_connection				= \"${DataResourceGroup}\"\n") // BUG, Resource Group is camel case in solution
									} else {
										templateBlock += fmt.Sprintf("\tis_manual_connection				= ${is_manual_connection		}\n") // BUG, Resource Group is camel case in solution
									}
								} else {
									if at1.DataTypeString == schema.TypeList.String() {
										templateBlock += fmt.Sprintf("\t%s		= ${%s}\n", n1, n1)
									} else {
										templateBlock += fmt.Sprintf("\t%s		= \"${%s}\"\n", n1, n1)
									}
								}
							} else {
								for n2, at2 := range at1.Attributes {
									if n2 == "name" && at2.ResourcePath != "azurerm_subnet.delegation.service_delegation.name" {
										continue
									} else {

										if n2 == "name" {

											vn := genVariableNameFromResourcePath(at2.ResourcePath)

											templateBlock += fmt.Sprintf("\t%s		= ${%s}\n", vn, vn)
										} else {
											if at2.DataTypeString == schema.TypeList.String() {
												templateBlock += fmt.Sprintf("\t%s		= ${%s}\n", n2, n2)
											} else {
												templateBlock += fmt.Sprintf("\t%s		= \"${%s}\"\n", n2, n2)
											}
										}

									}
								}

							}
						}
					}
				}
			}

		}

		templateBlock += "}\n"
	} else if gen.resourceName == "terraform_azurerm" {
		templateBlock += "terraform {\n"
		templateBlock += "	required_providers {\n"
		templateBlock += "		azurerm = {\n"
		templateBlock += fmt.Sprintf("			source =  \"${%s}\"\n", "terraform_azurerm_azurerm_source")
		templateBlock += fmt.Sprintf("			version = \"${%s}\"\n", "terraform_azurerm_azurerm_version")
		templateBlock += "		}\n"
		templateBlock += "		azapi = {\n"
		templateBlock += fmt.Sprintf("			source =  \"${%s}\"\n", "terraform_azurerm_azapi_source")
		templateBlock += fmt.Sprintf("			version = \"${%s}\"\n", "terraform_azurerm_azapi_version")
		templateBlock += "		}\n"
		templateBlock += "	}\n"
		templateBlock += "	backend \"azurerm\" {\n"
		templateBlock += "	}\n"
		templateBlock += "}\n"

		templateBlock += "provider \"azurerm\" {\n"
		templateBlock += "	features {\n"
		templateBlock += "	}\n"
		templateBlock += "}\n"
	} else if gen.resourceName == "devops_pipeline" {
		templateBlock += "name: $(connection)-$(Date:yyyyMMdd)$(Rev:.r)\n"
		templateBlock += "variables:\n"
		templateBlock += "  connection: 'sub-ret-d-001'\n"
		templateBlock += "trigger: none\n"
		templateBlock += "resources:\n"
		templateBlock += "  repositories:\n"
		templateBlock += "	- repository: Repo.Pipelines\n"
		templateBlock += "	  type: git\n"
		templateBlock += "	  name: Repo.Pipelines\n"
		templateBlock += "	  ref: refs/heads/main\n"
		templateBlock += "stages:\n"
		templateBlock += "- template: TerraformStages.yml@Repo.Pipelines\n"
		templateBlock += "  parameters:\n"
		templateBlock += "	ServiceShort      : storage_policy_test\n"
		templateBlock += "	serviceConnection : 'ServiceConnection.sub-ret-d-001'\n"
		templateBlock += "	EnvironmentShort  : dev\n"
	}

	return templateBlock
}

func (gen documentationGenerator) terraformModuleBlock() string {

	attributes := gen.injectAttributes()

	var moduleBlock string
	var appendBlock string

	moduleBlock += fmt.Sprintf("resource \"%s\" \"this\" {\n", gen.resourceName)
	moduleBlock += "\tname = local.name\n"
	for n, at := range attributes {
		if at.Computed {
			continue //Ignore computed values for time being
		}
		if n == "name" {
			fmt.Printf("terraformModuleBlock name at.DataTypeString: %v\n", len(at.PossibleValues))
			continue
		}
		if strings.Contains(n, "dlta_") { //Ignore any parameters that are for dlta, these are used elsewhere
			continue
		}
		if !at.IsBlock {
			if at.DataTypeString == schema.TypeList.String() {
				appendBlock += fmt.Sprintf("\t%s = var.%s\n", n, n)
			} else {
				moduleBlock += fmt.Sprintf("\t%s = var.%s\n", n, n)
			}
		} else {
			moduleBlock += fmt.Sprintf("\t%s {\n", n)

			//TODO multi level
			for k, a := range at.Attributes {

				if !a.IsBlock {
					if k == "name" {

						var cs string

						for i, v := range strings.Split(a.ResourcePath, ".") {
							if i == 0 {
								continue
							}
							cs += v
							if i < (len(strings.Split(a.ResourcePath, ".")) - 1) {
								cs += "_"
							}
						}
						moduleBlock += fmt.Sprintf("\t\tname = local.%s\n", cs)
					} else {
						moduleBlock += fmt.Sprintf("\t\t%s = var.%s\n", k, k)
					}
				} else {

					moduleBlock += fmt.Sprintf("\t\t%s {\n", k)
					for k2, a2 := range a.Attributes {
						if k2 == "name" {

							vn := genVariableNameFromResourcePath(a2.ResourcePath)
							moduleBlock += fmt.Sprintf("\t\t\tname = var.%s\n", vn)
						} else {
							moduleBlock += fmt.Sprintf("\t\t\t%s = var.%s\n", k2, k2)
						}

					}
					moduleBlock += "\t\t}\n"

				}

			}

			moduleBlock += "\t}\n"

		}

	}
	moduleBlock += appendBlock
	moduleBlock += "}\n"

	return moduleBlock
}

// TODO.....
func (gen documentationGenerator) terraformVariableBlock() string {

	attributes := gen.injectAttributes()

	var variableBlock string

	for n, at := range attributes {

		if n == "name" { // TODO  We need to work out the scenarios for this
			continue
		}

		if n != "dlta_terraform_template" && n != "dlta_naming_convention" && n != "dlta_terraform_module_name" && n != "dlta_terraform_is_data_source" {

			if !at.Computed { // Computed fields are never variables
				if !at.IsBlock {

					variableBlock += fmt.Sprintf("variable \"%s\" {\n", n)
					variableBlock += fmt.Sprintf("\tdescription = \"%s\"\n", at.Description)
					variableBlock += fmt.Sprintf("\ttype = %s\n", translateDataType(at.DataTypeString))
					if at.Default != "" {
						variableBlock += fmt.Sprintf("\tdefault = \"%s\"\n", at.Default)
					}
					variableBlock += "}\n"
				} else {

					//TODO multi level
					for n1, at1 := range at.Attributes {

						if !at1.IsBlock {
							if n1 == "name" { // TODO  We need to work out the scenarios for this
								continue
							}

							variableBlock += fmt.Sprintf("variable \"%s\" {\n", n1)
							variableBlock += fmt.Sprintf("\tdescription = \"%s\"\n", at1.Description)
							variableBlock += fmt.Sprintf("\ttype = %s\n", translateDataType(at1.DataTypeString))
							if at.Default != "" {
								variableBlock += fmt.Sprintf("\tdefault = \"%s\"\n", at1.Default)
							}
							variableBlock += "}\n"
						} else {
							for n2, at2 := range at1.Attributes {
								if n2 == "name" {
									var cs string

									for i, v := range strings.Split(at2.ResourcePath, ".") {
										if i == 0 {
											continue
										}
										cs += v
										if i < (len(strings.Split(at2.ResourcePath, ".")) - 1) {
											cs += "_"
										}
									}

									variableBlock += fmt.Sprintf("variable \"%s\" {\n", cs)
									variableBlock += fmt.Sprintf("\tdescription = \"%s\"\n", at2.Description)
									variableBlock += fmt.Sprintf("\ttype = %s\n", translateDataType(at2.DataTypeString))
									if at.Default != "" {
										variableBlock += fmt.Sprintf("\tdefault = \"%s\"\n", at2.Default)
									}
									variableBlock += "}\n"

								} else {
									variableBlock += fmt.Sprintf("variable \"%s\" {\n", n2)
									variableBlock += fmt.Sprintf("\tdescription = \"%s\"\n", at2.Description)
									variableBlock += fmt.Sprintf("\ttype = %s\n", translateDataType(at2.DataTypeString))
									if at.Default != "" {
										variableBlock += fmt.Sprintf("\tdefault = \"%s\"\n", at2.Default)
									}
									variableBlock += "}\n"
								}

							}

						}

					}

				}
			}
		}

	}

	return variableBlock
}

func (gen documentationGenerator) terraformLocalBlock() string {

	var localBlock string

	attributes := gen.injectAttributes()

	//TODO Use the global naming convention

	localBlock += "locals {\n"
	for n, at := range attributes {
		resShort1 := "dlta_vendor_asset_short_code"
		bizShort2 := "dlta_business_short_code"
		appShort3 := "dlta_application_short_code"
		envChar4 := "dlta_environment_char"
		locShort5 := "dlta_location_short_code"
		instId6 := "dlta_instance_id"

		if n == "name" {
			if gen.resourceName == "azurerm_storage_account" {

				localBlock += fmt.Sprintf("\tname = format(\"%%s%%s%%s%%s%%s%%s\",var.%s,var.%s,var.%s,var.%s,var.%s,var.%s)\n", resShort1, bizShort2, appShort3, envChar4, locShort5, instId6)
			} else {
				localBlock += fmt.Sprintf("\tname = format(\"%%s-%%s-%%s-%%s-%%s-%%s\",var.%s,var.%s,var.%s,var.%s,var.%s,var.%s)\n", resShort1, bizShort2, appShort3, envChar4, locShort5, instId6)

			}
		}

		if at.IsBlock {

			for k, a := range at.Attributes {

				if k == "name" {
					var variableName string
					var resourceName string

					// fmt.Printf("a.ResourcePath: %v\n", a.ResourcePath)
					for i, v := range strings.Split(a.ResourcePath, ".") {
						if i == 0 {
							continue
						}
						if i == 1 {
							resourceName = v
						}
						variableName += v
						if i < (len(strings.Split(a.ResourcePath, ".")) - 1) {
							variableName += "_"
						}
					}

					fmt.Printf("resourceName: %v\n", resourceName)
					resShort1 = getResourceShortCode(resourceName)
					fmt.Printf("resShort1: %v\n", resShort1)
					localBlock += fmt.Sprintf("\t%s = format(\"%%s-%%s-%%s-%%s-%%s-%%s\",\"%s\",var.%s,var.%s,var.%s,var.%s,var.%s)\n", variableName, resShort1, bizShort2, appShort3, envChar4, locShort5, instId6)

				}

			}
		}
	}
	localBlock += "}\n"

	return localBlock
}

func (gen documentationGenerator) getPalletProp(at attribute, name string) PaletteProp {

	var pp PaletteProp
	var flattenName string

	flattenName = ""
	pp.ID = name
	pp.Type = translateDataType(at.DataTypeString)
	pp.Name = convertNameToLabel(name)
	pp.Disabled = false
	pp.FlattenName = &flattenName
	pp.CurrentValue = initiaiseAttribute(at.DataTypeString)

	switch name {
	case "name":
		if len(at.PossibleOptions) > 0 || len(at.PossibleValues) > 0 { // This is not a generated name

			vn := genVariableNameFromResourcePath(at.ResourcePath)
			flattenName = ""
			pp.ID = vn
			pp.Type = translateDataType(at.DataTypeString)
			pp.Name = convertNameToLabel(vn)
			pp.Disabled = false
			pp.FlattenName = &flattenName
			pp.CurrentValue = initiaiseAttribute(at.DataTypeString)

			if len(at.PossibleValues) > 0 {

				var keyVal KeyValue

				for i := 0; i < len(at.PossibleValues); i++ {
					keyVal.Key = at.PossibleValues[i]
					keyVal.Value = at.PossibleValues[i]

					pp.Options = append(pp.Options, keyVal)
				}

				if at.DataTypeString == schema.TypeList.String() {
					pp.Type = "checkboxes"
					var a []string
					pp.CurrentValue = a
				} else {
					pp.Type = "select"
					pp.CurrentValue = ""
				}

			} else {
				fmt.Printf("FSNAME: %v\n", name)
			}

			if at.DataTypeString == "TypeBool" {
				pp.Type = "checkbox"
			}
		}
	case "AssetType":
		flattenName = "AssetType"
		pp.ID = "AssetType"
		pp.Type = "string"
		pp.Name = "Asset Type:"
		pp.Disabled = true
		pp.FlattenName = &flattenName
		pp.CurrentValue = gen.resourceName
	case "Name": // This is the name of the asset as dropped onto the canvas
		validators := make(NameValue)
		validators["required"] = true
		validators["minLength"] = 3
		flattenName = "Name"
		pp.ID = "name"
		pp.Type = "string"
		pp.Name = "Name:"
		pp.Disabled = true
		pp.FlattenName = &flattenName
		pp.CurrentValue = initiaiseAttribute("TypeString")
		pp.Validators = validators
	case "location":
		flattenName = ""
		pp.ID = name
		pp.Type = translateDataType(at.DataTypeString)
		pp.Name = convertNameToLabel(name)
		pp.Disabled = false
		pp.FlattenName = &flattenName
		pp.CurrentValue = initiaiseAttribute(at.DataTypeString)

		for i := 0; i < len(dlta_location_options); i++ {
			pp.Options = append(pp.Options, dlta_location_options[i])
		}

		if len(dlta_location_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = dlta_location_options[0].Value
	case "dlta_application_short_code":
		flattenName = ""
		pp.ID = name
		pp.Type = translateDataType(at.DataTypeString)
		pp.Name = convertNameToLabel(name)
		pp.Disabled = false
		pp.FlattenName = &flattenName
		pp.CurrentValue = initiaiseAttribute(at.DataTypeString)
		for i := 0; i < len(dlta_application_short_code_options); i++ {
			pp.Options = append(pp.Options, dlta_application_short_code_options[i])
		}

		if len(dlta_application_short_code_options) > 0 {
			pp.Type = "select"
		}
		pp.CurrentValue = dlta_application_short_code_options[0].Value
	case "dlta_business_short_code":
		flattenName = ""
		pp.ID = name
		pp.Type = translateDataType(at.DataTypeString)
		pp.Name = convertNameToLabel(name)
		pp.Disabled = false
		pp.FlattenName = &flattenName
		pp.CurrentValue = initiaiseAttribute(at.DataTypeString)
		for i := 0; i < len(dlta_business_short_code_options); i++ {
			pp.Options = append(pp.Options, dlta_business_short_code_options[i])
		}

		if len(dlta_business_short_code_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = dlta_business_short_code_options[0].Value
	case "dlta_environment_char":
		for i := 0; i < len(dlta_environment_char_options); i++ {
			pp.Options = append(pp.Options, dlta_environment_char_options[i])
		}

		if len(dlta_environment_char_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = dlta_environment_char_options[0].Value

	case "dlta_instance_id":
		for i := 0; i < len(dlta_instance_id_options); i++ {
			pp.Options = append(pp.Options, dlta_instance_id_options[i])
		}

		if len(dlta_instance_id_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = dlta_instance_id_options[0].Value
	case "dlta_location_short_code":
		for i := 0; i < len(dlta_location_short_code_options); i++ {
			pp.Options = append(pp.Options, dlta_location_short_code_options[i])
		}

		if len(dlta_location_short_code_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = dlta_location_short_code_options[0].Value
	case "dlta_vendor_asset_short_code":
		pp.CurrentValue = gen.ShortCode
		pp.Disabled = true
	case "dlta_terraform_template":
		pp.CurrentValue = gen.terraformTemplateBlock()
		pp.Disabled = true
		pp.Type = "textarea"

	case "dlta_terraform_is_data_source":
		if gen.isDataSource {
			pp.CurrentValue = "data"
		} else {
			pp.CurrentValue = "resource"
		}

		pp.Disabled = true

	case "resource_group_name":
		if gen.isDataSource {

			pp = PaletteProp{}
			pp.ID = "DataResourceGroup"
			pp.Type = "string"
			pp.Name = "Resource Group:"
			pp.Disabled = false
			pp.FlattenName = &flattenName
			pp.CurrentValue = nil
		} else {
			flattenName = "ResourceGroups"

			pp = PaletteProp{}
			pp.ID = "ResourceGroup"
			pp.Type = "string"
			pp.Name = "Resource Group:"
			pp.Disabled = true
			pp.FlattenName = &flattenName
			pp.CurrentValue = nil
		}

		// creation.Props = append(creation.Props, palletItem)
	case "terraform_azurerm_azapi_source":
		for i := 0; i < len(terraform_azurerm_azapi_source_options); i++ {
			pp.Options = append(pp.Options, terraform_azurerm_azapi_source_options[i])
		}

		if len(terraform_azurerm_azapi_source_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = terraform_azurerm_azapi_source_options[0].Value
	case "terraform_azurerm_azapi_version":
		for i := 0; i < len(terraform_azurerm_azapi_version_options); i++ {
			pp.Options = append(pp.Options, terraform_azurerm_azapi_version_options[i])
		}

		if len(terraform_azurerm_azapi_version_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = terraform_azurerm_azapi_version_options[0].Value
	case "terraform_azurerm_azurerm_source":
		for i := 0; i < len(terraform_azurerm_azurerm_source_options); i++ {
			pp.Options = append(pp.Options, terraform_azurerm_azurerm_source_options[i])
		}

		if len(terraform_azurerm_azurerm_source_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = terraform_azurerm_azurerm_source_options[0].Value
	case "terraform_azurerm_azurerm_version":
		for i := 0; i < len(terraform_azurerm_azurerm_version_options); i++ {
			pp.Options = append(pp.Options, terraform_azurerm_azurerm_version_options[i])
		}

		if len(terraform_azurerm_azurerm_version_options) > 0 {
			pp.Type = "select"
		}

		pp.CurrentValue = terraform_azurerm_azurerm_version_options[0].Value
	case "dlta_naming_convention":
		pp.CurrentValue = gen.NamingConvention
		pp.Disabled = true
	default:
		if len(at.PossibleValues) > 0 {

			var keyVal KeyValue

			for i := 0; i < len(at.PossibleValues); i++ {
				keyVal.Key = at.PossibleValues[i]
				keyVal.Value = at.PossibleValues[i]

				pp.Options = append(pp.Options, keyVal)
			}

			if at.DataTypeString == schema.TypeList.String() {
				pp.Type = "checkboxes"
				var a []string
				pp.CurrentValue = a
			} else {
				pp.Type = "select"
				// pp.CurrentValue = at.PossibleValues[0]
			}

		} else {
			fmt.Printf("FSNAME: %v\n", name)
		}

		if at.DataTypeString == "TypeBool" {
			pp.Type = "checkbox"
		}
	}

	return pp
}

func (gen documentationGenerator) dltaPalletteCodeBlock() string {

	attributes := gen.injectAttributes()
	var dltaPalletteCodeBlock string
	var palletItem PaletteProp
	var creation Creator

	//generateInsertString
	dltaPalletteCodeBlock += "insert into core.infra_asset (\n"
	dltaPalletteCodeBlock += "				id, 		guid, infra_id, name,	label,	type,	active, 	addable,	asset_type,	reflect_type, 	palette_design, form_fields, 	attributes, created_at, updated_at, deleted_at, updated_by,	rank, 	has_cost, svg_icon) values (\n"
	dltaPalletteCodeBlock += fmt.Sprintf("	DEFAULT, 	'%s', 1, 		'%s', 	'%s', 	'', 	true, 		true, 		'%s',		'none', 		null, 			'{}', 			null, 		now(), 		now(), 		null, 		1,			14, 	false, 		''	\n", uuid.New().String(), gen.resourceName, gen.resourceName, gen.resourceName)
	dltaPalletteCodeBlock += ");\n"
	//Start insert

	//TODO Move to init area
	//Enum for attribute type e.g. autoGenName or inputProperty

	creation.CreateFunction = gen.resourceName

	//getPalletItem()
	palletItem = gen.getPalletProp(attribute{}, "AssetType")

	// var flattenName string

	creation.Props = append(creation.Props, palletItem)

	if gen.resourceName != "terraform_azurerm" && gen.resourceName != "devops_pipeline" && !gen.isDataSource {

		palletItem = gen.getPalletProp(attribute{}, "Name")
		creation.Props = append(creation.Props, palletItem)
	}

	for n, fs := range attributes {

		if n == "name" {
			continue
		}
		// palletItem = PaletteProp{}

		if n == "location" {
			palletItem = gen.getPalletProp(fs, "location")
		} else if n == "dlta_application_short_code" {
			palletItem = gen.getPalletProp(fs, "dlta_application_short_code")
		} else if n == "dlta_business_short_code" {
			palletItem = gen.getPalletProp(fs, "dlta_business_short_code")
		} else if n == "dlta_environment_char" {
			palletItem = gen.getPalletProp(fs, "dlta_environment_char")
		} else if n == "dlta_instance_id" {
			palletItem = gen.getPalletProp(fs, "dlta_instance_id")
		} else if n == "dlta_location_short_code" {
			palletItem = gen.getPalletProp(fs, "dlta_location_short_code")
		} else if n == "dlta_vendor_asset_short_code" {
			palletItem = gen.getPalletProp(fs, "dlta_vendor_asset_short_code")
		} else if n == "dlta_terraform_template" {
			palletItem = gen.getPalletProp(fs, "dlta_terraform_template")
		} else if n == "dlta_terraform_is_data_source" {
			palletItem = gen.getPalletProp(fs, "dlta_terraform_is_data_source")
		} else if n == "resource_group_name" {
			palletItem = gen.getPalletProp(fs, "resource_group_name")
		} else if n == "terraform_azurerm_azapi_source" {
			palletItem = gen.getPalletProp(fs, "terraform_azurerm_azapi_source")
		} else if n == "terraform_azurerm_azapi_version" {
			palletItem = gen.getPalletProp(fs, "terraform_azurerm_azapi_version")
		} else if n == "terraform_azurerm_azurerm_source" {
			palletItem = gen.getPalletProp(fs, "terraform_azurerm_azurerm_source")
		} else if n == "terraform_azurerm_azurerm_version" {
			palletItem = gen.getPalletProp(fs, "terraform_azurerm_azurerm_version")
		} else if n == "dlta_naming_convention" {
			palletItem = gen.getPalletProp(fs, "dlta_naming_convention")
		} else {
			palletItem = gen.getPalletProp(fs, n)
		}

		if fs.IsBlock {
			for n1, at := range fs.Attributes {
				if !at.IsBlock {
					palletItem = gen.getPalletProp(at, n1)
					creation.Props = append(creation.Props, palletItem)
				} else {
					for n2, at2 := range at.Attributes {
						palletItem = gen.getPalletProp(at2, n2)
						creation.Props = append(creation.Props, palletItem)
					}
				}

			}
		} else {
			creation.Props = append(creation.Props, palletItem)
		}
	}

	dltaPalletteCodeBlock += fmt.Sprintf("UPDATE core.infra_asset SET has_cost= %s,\nform_fields = '", strconv.FormatBool(false))

	dltaPalletteCodeBlock += writeJson(creation)

	dltaPalletteCodeBlock += fmt.Sprintf("'\nwhere asset_type = '%s'", gen.resourceName)
	//Finish insert
	dltaPalletteCodeBlock += ";"

	return dltaPalletteCodeBlock
}

func (gen documentationGenerator) terraformOutputBlock() string {

	var outputBlock string

	if gen.resource != nil {

		attributes := gen.getAllOutputAttributes(gen.resource.Schema, attribute{}, false, gen.resourceName)

		for k, _ := range attributes {
			outputBlock += "output \"" + k + "\" {\n"
			outputBlock += "\tvalue = " + gen.resourceName + ".this." + k + "\n"
			outputBlock += "}\n"
		}

	}

	return outputBlock
}

func (gen documentationGenerator) getResourceNamingConvention(resourceName string, isDataSource bool) string {

	// menu := make(map[string][]string)

	//	Angular

	var static string
	var delim string
	var fields []string
	var prefix string

	resourceSpecificNaming := map[string]namingStruct{
		"terraform_azurerm":                  {Delimiter: "-", StaticName: "terraform_azurerm"},
		"azurerm_subscription":               {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_resource_group":             {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_windows_web_app":            {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_windows_function_app":       {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_service_plan":               {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_storage_account":            {Delimiter: "", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_cdn_frontdoor_profile":      {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_cdn_frontdoor_endpoint":     {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_cdn_frontdoor_origin_group": {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_cdn_frontdoor_origin":       {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_key_vault_access_policy":    {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_key_vault":                  {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_private_endpoint":           {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_virtual_network":            {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_subnet":                     {Delimiter: "-", StaticName: "", Prefix: "", IsDataSource: false, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
	}

	dataSourceSpecificNaming := map[string]namingStruct{
		"azurerm_subnet":                {Delimiter: "-", StaticName: "", Prefix: "ds", IsDataSource: true, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
		"azurerm_key_vault_certificate": {Delimiter: "-", StaticName: "", Prefix: "ds", IsDataSource: true, Fields: []string{"dlta_vendor_asset_short_code", "dlta_business_short_code", "dlta_application_short_code", "dlta_environment_char", "dlta_location_short_code", "dlta_instance_id"}},
	}

	if isDataSource {
		static = dataSourceSpecificNaming[resourceName].StaticName
		delim = dataSourceSpecificNaming[resourceName].Delimiter
		fields = dataSourceSpecificNaming[resourceName].Fields
		prefix = dataSourceSpecificNaming[resourceName].Prefix
	} else {
		static = resourceSpecificNaming[resourceName].StaticName
		delim = resourceSpecificNaming[resourceName].Delimiter
		fields = resourceSpecificNaming[resourceName].Fields
		prefix = resourceSpecificNaming[resourceName].Prefix
	}

	returnString := ""

	if static != "" {
		returnString = static
	} else {
		if prefix != "" {
			returnString += prefix + delim
		}
		for i := 0; i < len(fields); i++ {
			returnString += fmt.Sprintf("${%v}", fields[i])
			if i < (len(fields) - 1) {
				returnString += delim
			}
		}
	}

	return returnString
}

func genVariableNameFromResourcePath(rp string) string {
	var cs string

	for i, v := range strings.Split(rp, ".") {

		cs += v
		if i < (len(strings.Split(rp, ".")) - 1) {
			cs += "_"
		}
	}

	return cs
}

func getResourceShortCode(resourceName string) string {
	resourceNames := strings.Split(resourceName, "_")

	var resourceShortCode string
	parts := len(resourceName)
	for i, v := range resourceNames {
		if parts == 1 {
			if i == 0 {
				continue
			}
		}

		resourceShortCode += v[0:1]
	}

	return resourceShortCode
}

func translateDataType(terraType string) string {

	switch terraType {
	case "TypeString":
		return "string"
	case "TypeBool":
		return "bool"
	case "TypeInt":
		return "number"
	case "TypeFloat":
		return "number"
	case "TypeList":
		return "list"
	case "TypeMap":
		return "map"
	default:
		return "property undefiend"
	}
}

func writeDebugJson(v any) string {

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("Error: %s", err)

	}
	color.Red(string(b))
	return string(b)
}

func writeJson(v any) string {

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Printf("Error: %s", err)

	}
	return string(b)
}

func writeDebug(input string) {
	color.Red(input)
}

func isBlock(element interface{}) bool {
	attribute, isSchema := element.(*schema.Schema)
	var isResource bool
	if isSchema {
		_, isResource = attribute.Elem.(*schema.Resource)
	}

	return isResource
}

func convertNameToLabel(name string) string {
	nameElements := strings.Split(name, "_")

	var label string
	// dlta_application_short_code

	for i, v := range nameElements {
		if v == "dlta" { //Ignore the dlta part of the mame
			continue
		} else {
			label += strings.Title(v)
			if i < (len(nameElements) - 1) {
				label += " "
			}

		}

	}
	return label
}

func cloneSchemaToAttributes(a *attribute, s *schema.Schema, isBlock bool, parentPath string, fieldName string) {

	a.Description = s.Description
	a.IsBlock = isBlock
	a.MaxItems = s.MaxItems
	a.MinItems = s.MinItems
	a.Optional = s.Optional
	a.Computed = s.Computed
	a.Required = s.Required
	a.ForceNew = s.ForceNew
	//a.PossibleValues  = s.PossibleValues
	//a.PossibleOptions = s.PossibleOptions
	a.DataTypeString = s.Type.String()
	//a.Default         = s.Default //TODO Find out how this works  SchemaDefaultFunc
	a.ConflictsWith = s.ConflictsWith
	a.ResourcePath = parentPath + "." + fieldName
}

func cloneSchemaToAttributesSummary(a *attributeSummary, s *schema.Schema, isBlock bool, parentPath string, fieldName string) {

	a.IsBlock = isBlock
	a.DataTypeString = s.Type.String()
	a.ResourcePath = parentPath + "." + fieldName
}

func (gen documentationGenerator) sortFields(input map[string]*schema.Schema) []string {
	fieldNames := make([]string, 0)
	for field := range input {
		fieldNames = append(fieldNames, field)
	}
	sort.Strings(fieldNames)
	return fieldNames
}

func patchPossibleValuesFn() {
	gomonkey.ApplyFunc(help.StringInSlice,
		func(valid []string, ignoreCase bool) schema.SchemaValidateFunc { //nolint:staticcheck
			return func(i interface{}, k string) (warnings []string, errors []error) {
				var res []string // must have a copy
				res = append(res, valid...)
				return res, nil
			}
		},
	)
}

func StringInSlice() {
	gomonkey.ApplyFunc(validation.StringInSlice,
		func(valid []string, ignoreCase bool) func(interface{}, string) ([]string, []error) { //nolint:staticcheck
			return func(i interface{}, k string) (warnings []string, errors []error) {
				var res []string // must have a copy
				res = append(res, valid...)
				return res, nil
			}
		},
	)
}

func init() {
	patchPossibleValuesFn()
	StringInSlice()
}

func getSchemaPossibleValues(item *schema.Schema) []string {
	if item.ValidateFunc != nil {
		// check if it is StringsInSlice
		pc := reflect.ValueOf(item.ValidateFunc).Pointer()
		fn := runtime.FuncForPC(pc)
		fnName := fn.Name()
		// seems different go version behaviors different
		if strings.Contains(fnName, "StringInSlice") || strings.Contains(fnName, "patchPossibleValuesFn") {

			values, _ := item.ValidateFunc(nil, "")
			return values
		}
	}
	return nil
}

func initiaiseAttribute(terraType string) interface{} {

	switch terraType {
	case "TypeString":
		var r = ""
		return writeJson(r)
	case "TypeBool":
		var r = false
		return r
	case "TypeInt":
		var r = 0
		return r
	case "TypeFloat":
		var r = 0.0
		return r
	case "TypeList":
		var r []string = []string{""}
		return r
	case "TypeMap":
		var r map[string]string
		return r
	default:
		return nil
	}
}
