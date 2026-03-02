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

type MakeUsecase struct{}

// Signature The name and signature of the console command.
func (receiver *MakeUsecase) Signature() string {
	return "make:usecase"
}

// Description The console command description.
func (receiver *MakeUsecase) Description() string {
	return "Create a new usecase with interface"
}

// Extend The console command extend.
func (receiver *MakeUsecase) Extend() command.Extend {
	return command.Extend{
		Category: "make",
	}
}

// Handle Execute the console command.
func (receiver *MakeUsecase) Handle(ctx console.Context) error {
	usecaseName := ctx.Argument(0)
	if usecaseName == "" {
		facades.Log().Error("Usecase name is required")
		return fmt.Errorf("usecase name is required")
	}

	// Validate PascalCase before conversion
	if !receiver.isPascalCase(usecaseName) {
		facades.Log().Error("Usecase name must be in PascalCase (e.g., UserUsecase)")
		return fmt.Errorf("usecase name must be in PascalCase")
	}

	// Convert to proper interface name format
	interfaceName := receiver.convertToInterfaceName(usecaseName)
	usecaseStructName := receiver.getUsecaseName(interfaceName)

	// Create usecases directory if it doesn't exist
	usecasesDir := "app/usecases"
	if err := os.MkdirAll(usecasesDir, 0755); err != nil {
		facades.Log().Error("Failed to create usecases directory: " + err.Error())
		return err
	}

	// Check usecase file first - reject if exists
	usecaseFileName := receiver.toSnakeCase(usecaseStructName)
	usecaseFilePath := filepath.Join(usecasesDir, usecaseFileName+".go")

	if _, err := os.Stat(usecaseFilePath); err == nil {
		facades.Log().Error("Usecase file already exists: " + usecaseFilePath)
		return fmt.Errorf("usecase file already exists: %s", usecaseFilePath)
	}

	// Handle interface file
	interfaceFilePath := filepath.Join(usecasesDir, "interface.go")
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

	// Create usecase file
	if err := receiver.createUsecaseFile(usecaseFilePath, usecaseStructName, interfaceName); err != nil {
		facades.Log().Error("Failed to create usecase file: " + err.Error())
		return err
	}

	facades.Log().Info("Usecase created successfully: " + usecaseFilePath)
	facades.Log().Info("Interface processed successfully: " + interfaceName + " in " + interfaceFilePath)
	return nil
}

func (receiver *MakeUsecase) convertToInterfaceName(name string) string {
	// If already ends with UsecaseInterface, keep as is
	if strings.HasSuffix(name, "UsecaseInterface") {
		return name
	}

	// If ends with Usecase, append Interface
	if strings.HasSuffix(name, "Usecase") {
		return name + "Interface"
	}

	return name + "UsecaseInterface"
}

func (receiver *MakeUsecase) getUsecaseName(interfaceName string) string {
	// Remove "Interface" suffix to get usecase name
	// Ex : UserUsecaseInterface -> UserUsecase
	return strings.TrimSuffix(interfaceName, "Interface")
}

func (receiver *MakeUsecase) toSnakeCase(s string) string {
	// Insert underscore before uppercase letters (except the first one)
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(s, "${1}_${2}")

	// Convert to lowercase
	return strings.ToLower(snake)
}

func (receiver *MakeUsecase) isPascalCase(s string) bool {
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

func (receiver *MakeUsecase) interfaceExists(filePath, interfaceName string) (bool, error) {
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

func (receiver *MakeUsecase) addInterfaceToFile(filePath, interfaceName string) error {
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

func (receiver *MakeUsecase) generateInterfaceAST(interfaceName string) *ast.GenDecl {
	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Usecase")
	methodName := "Execute" + baseName

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

func (receiver *MakeUsecase) generateInterfaceFileContent(interfaceName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("usecase-interface.plain.stub")
	if err != nil {
		return "", err
	}

	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Usecase")
	methodName := "Execute" + baseName

	// Replace placeholders with actual interface name and method
	content := strings.ReplaceAll(string(stubContent), "DummyUsecaseInterface", interfaceName)
	content = strings.ReplaceAll(content, "DummyMethod", methodName)

	return content, nil
}

func (receiver *MakeUsecase) createUsecaseFile(filePath, usecaseName, interfaceName string) error {
	// Generate usecase content using stub
	structName := receiver.toCamelCase(usecaseName)
	methodName := receiver.extractMethodName(interfaceName)

	content, err := receiver.generateUsecaseContent(structName, usecaseName, interfaceName, methodName)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (receiver *MakeUsecase) toCamelCase(s string) string {
	if s == "" {
		return ""
	}

	// Convert first character to lowercase
	return strings.ToLower(s[:1]) + s[1:]
}

func (receiver *MakeUsecase) extractMethodName(interfaceName string) string {
	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Usecase")
	return "Execute" + baseName
}

func (receiver *MakeUsecase) generateUsecaseContent(structName, usecaseName, interfaceName, methodName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("usecase.plain.stub")
	if err != nil {
		return "", err
	}

	// Replace placeholders with actual values
	content := strings.ReplaceAll(string(stubContent), "dummyUsecase", structName)
	content = strings.ReplaceAll(content, "DummyUsecase", usecaseName)
	content = strings.ReplaceAll(content, "DummyUsecaseInterface", interfaceName)
	content = strings.ReplaceAll(content, "DummyMethod", methodName)

	return content, nil
}
