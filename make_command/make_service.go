package make_command

import (
	"fmt"
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"
	"github.com/goravel/framework/facades"
	"github.com/supportapplibs/go-lib/stubs"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type MakeService struct{}

// Signature The name and signature of the console command.
func (receiver *MakeService) Signature() string {
	return "make:service"
}

// Description The console command description.
func (receiver *MakeService) Description() string {
	return "Create a new service with interface"
}

// Extend The console command extend.
func (receiver *MakeService) Extend() command.Extend {
	return command.Extend{
		Category: "make",
	}
}

// Handle Execute the console command.
func (receiver *MakeService) Handle(ctx console.Context) error {
	serviceName := ctx.Argument(0)
	if serviceName == "" {
		facades.Log().Error("Service name is required")
		return fmt.Errorf("service name is required")
	}

	// Validate PascalCase before conversion
	if !receiver.isPascalCase(serviceName) {
		facades.Log().Error("Service name must be in PascalCase (e.g., UserService)")
		return fmt.Errorf("service name must be in PascalCase")
	}

	// Convert to proper interface name format
	interfaceName := receiver.convertToInterfaceName(serviceName)
	serviceStructName := receiver.getServiceName(interfaceName)

	// Create services directory if it doesn't exist
	servicesDir := "app/services"
	if err := os.MkdirAll(servicesDir, 0755); err != nil {
		facades.Log().Error("Failed to create services directory: " + err.Error())
		return err
	}

	// Check service file first - reject if exists
	serviceFileName := receiver.toSnakeCase(serviceStructName)
	serviceFilePath := filepath.Join(servicesDir, serviceFileName+".go")

	if _, err := os.Stat(serviceFilePath); err == nil {
		facades.Log().Error("Service file already exists: " + serviceFilePath)
		return fmt.Errorf("service file already exists: %s", serviceFilePath)
	}

	// Handle interface file
	interfaceFilePath := filepath.Join(servicesDir, "interface.go")
	if _, err := os.Stat(interfaceFilePath); err == nil {
		// File exists, check if interface already exists
		exists, err := receiver.interfaceExists(interfaceFilePath, interfaceName)
		if err != nil {
			facades.Log().Error("Failed to check existing interfaces: " + err.Error())
			return err
		}

		if !exists {
			// Add interface to existing file
			if err := receiver.addInterfaceToFile(interfaceFilePath, interfaceName); err != nil {
				facades.Log().Error("Failed to add interface to file: " + err.Error())
				return err
			}
		}
	} else {
		// Create new interface file
		content, err := receiver.generateInterfaceFileContent(interfaceName)
		if err != nil {
			facades.Log().Error("Failed to generate interface content: " + err.Error())
			return err
		}

		if err := os.WriteFile(interfaceFilePath, []byte(content), 0644); err != nil {
			facades.Log().Error("Failed to create interface file: " + err.Error())
			return err
		}
	}

	// Create service file
	if err := receiver.createServiceFile(serviceFilePath, serviceStructName, interfaceName); err != nil {
		facades.Log().Error("Failed to create service file: " + err.Error())
		return err
	}

	facades.Log().Info("Service created successfully: " + serviceFilePath)
	facades.Log().Info("Interface processed successfully: " + interfaceName + " in " + interfaceFilePath)
	return nil
}

func (receiver *MakeService) convertToInterfaceName(name string) string {
	// If already ends with ServiceInterface, keep as is
	if strings.HasSuffix(name, "ServiceInterface") {
		return name
	}

	// If ends with Service, append Interface
	if strings.HasSuffix(name, "Service") {
		return name + "Interface"
	}

	return name + "ServiceInterface"
}

func (receiver *MakeService) getServiceName(interfaceName string) string {
	// Remove "Interface" suffix to get service name
	// Ex : TestingDataServiceInterface -> TestingDataService
	return strings.TrimSuffix(interfaceName, "Interface")
}

func (receiver *MakeService) toSnakeCase(s string) string {
	// Insert underscore before uppercase letters (except the first one)
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(s, "${1}_${2}")

	// Convert to lowercase
	return strings.ToLower(snake)
}

func (receiver *MakeService) isPascalCase(s string) bool {
	if s == "" {
		return false
	}

	// Must start with uppercase letter
	if !unicode.IsUpper(rune(s[0])) {
		return false
	}

	// Only letters and numbers allowed, no underscores or spaces
	for _, char := range s {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
	}

	return true
}

func (receiver *MakeService) interfaceExists(filePath, interfaceName string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return false, err
	}

	for _, decl := range node.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == interfaceName {
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

func (receiver *MakeService) addInterfaceToFile(filePath, interfaceName string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		return err
	}

	// Generate new interface AST
	newInterface := receiver.generateInterfaceAST(interfaceName)

	// Add the new interface to the file
	node.Decls = append(node.Decls, newInterface)

	// Format and write back to file
	var buf strings.Builder
	if err := format.Node(&buf, fset, node); err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(buf.String()), 0644)
}

func (receiver *MakeService) generateInterfaceAST(interfaceName string) *ast.GenDecl {
	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Service")
	methodName := "Get" + baseName

	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{
			&ast.TypeSpec{
				Name: ast.NewIdent(interfaceName),
				Type: &ast.InterfaceType{
					Methods: &ast.FieldList{
						List: []*ast.Field{
							{
								Names: []*ast.Ident{ast.NewIdent(methodName)},
								Type: &ast.FuncType{
									Params: &ast.FieldList{
										List: []*ast.Field{
											{
												Names: []*ast.Ident{ast.NewIdent("ctx")},
												Type: &ast.SelectorExpr{
													X:   ast.NewIdent("context"),
													Sel: ast.NewIdent("Context"),
												},
											},
										},
									},
									Results: &ast.FieldList{
										List: []*ast.Field{
											{
												Names: []*ast.Ident{ast.NewIdent("result")},
												Type:  ast.NewIdent("interface{}"),
											},
											{
												Names: []*ast.Ident{ast.NewIdent("err")},
												Type:  ast.NewIdent("error"),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (receiver *MakeService) generateInterfaceFileContent(interfaceName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("interface.plain.stub")
	if err != nil {
		return "", err
	}

	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Service")
	methodName := "Get" + baseName

	// Replace placeholders with actual interface name and method
	content := strings.ReplaceAll(string(stubContent), "DummyInterface", interfaceName)
	content = strings.ReplaceAll(content, "DummyMethod", methodName)

	return content, nil
}

func (receiver *MakeService) createServiceFile(filePath, serviceName, interfaceName string) error {
	// Generate service content using stub
	structName := receiver.toCamelCase(serviceName)
	methodName := receiver.extractMethodName(interfaceName)

	content, err := receiver.generateServiceContent(structName, serviceName, interfaceName, methodName)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (receiver *MakeService) toCamelCase(s string) string {
	if s == "" {
		return ""
	}

	// Convert first character to lowercase
	return strings.ToLower(s[:1]) + s[1:]
}

func (receiver *MakeService) extractMethodName(interfaceName string) string {
	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Service")
	return "Get" + baseName
}

func (receiver *MakeService) generateServiceContent(structName, serviceName, interfaceName, methodName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("service.plain.stub")
	if err != nil {
		return "", err
	}

	// Replace placeholders with actual values
	content := strings.ReplaceAll(string(stubContent), "dummyService", structName)
	content = strings.ReplaceAll(content, "DummyService", serviceName)
	content = strings.ReplaceAll(content, "DummyServiceInterface", interfaceName)
	content = strings.ReplaceAll(content, "DummyMethod", methodName)

	return content, nil
}
