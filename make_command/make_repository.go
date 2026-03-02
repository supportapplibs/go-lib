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

type MakeRepository struct{}

// Signature The name and signature of the console command.
func (receiver *MakeRepository) Signature() string {
	return "make:repo"
}

// Description The console command description.
func (receiver *MakeRepository) Description() string {
	return "Create a new repository with interface and model"
}

// Extend The console command extend.
func (receiver *MakeRepository) Extend() command.Extend {
	return command.Extend{
		Category: "make",
	}
}

// Handle Execute the console command.
func (receiver *MakeRepository) Handle(ctx console.Context) error {
	repoName := ctx.Argument(0)
	if repoName == "" {
		facades.Log().Error("Repository name is required")
		return fmt.Errorf("repository name is required")
	}

	// Validate PascalCase before conversion
	if !receiver.isPascalCase(repoName) {
		facades.Log().Error("Repository name must be in PascalCase (e.g., UserRepository)")
		return fmt.Errorf("repository name must be in PascalCase")
	}

	// Convert to proper interface name format
	interfaceName := receiver.convertToInterfaceName(repoName)
	repoStructName := receiver.getRepositoryName(interfaceName)
	modelName := receiver.extractBaseName(interfaceName)

	// Create repositories directory if it doesn't exist
	repoDir := "app/repositories"
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		facades.Log().Error("Failed to create repositories directory: " + err.Error())
		return err
	}

	// Create models directory if it doesn't exist
	modelsDir := "app/models"
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		facades.Log().Error("Failed to create models directory: " + err.Error())
		return err
	}

	// Check repository file first - reject if exists
	repoFileName := receiver.toSnakeCase(repoStructName)
	repoFilePath := filepath.Join(repoDir, repoFileName+".go")

	if _, err := os.Stat(repoFilePath); err == nil {
		facades.Log().Error("Repository file already exists: " + repoFilePath)
		return fmt.Errorf("repository file already exists: %s", repoFilePath)
	}

	// Handle interface file
	interfaceFilePath := filepath.Join(repoDir, "interface.go")
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

	// Create model file
	modelFileName := receiver.toSnakeCase(modelName)
	modelFilePath := filepath.Join(modelsDir, modelFileName+".go")

	// Check if model already exists, if not create it
	if _, err := os.Stat(modelFilePath); os.IsNotExist(err) {
		if err := receiver.createModelFile(modelFilePath, modelName); err != nil {
			facades.Log().Error("Failed to create model file: " + err.Error())
			return err
		}
		facades.Log().Info("Model created successfully: " + modelFilePath)
	} else {
		facades.Log().Info("Model already exists, skipping: " + modelFilePath)
	}

	// Create repository file
	if err := receiver.createRepositoryFile(repoFilePath, repoStructName, interfaceName); err != nil {
		facades.Log().Error("Failed to create repository file: " + err.Error())
		return err
	}

	facades.Log().Info("Repository created successfully: " + repoFilePath)
	facades.Log().Info("Interface processed successfully: " + interfaceName + " in " + interfaceFilePath)
	return nil
}

func (receiver *MakeRepository) convertToInterfaceName(name string) string {
	// If already ends with RepositoryInterface, keep as is
	if strings.HasSuffix(name, "RepositoryInterface") {
		return name
	}

	// If ends with Repository, append Interface
	if strings.HasSuffix(name, "Repository") {
		return name + "Interface"
	}

	return name + "RepositoryInterface"
}

func (receiver *MakeRepository) getRepositoryName(interfaceName string) string {
	// Remove "Interface" suffix to get repository name
	// Ex : UserRepositoryInterface -> UserRepository
	return strings.TrimSuffix(interfaceName, "Interface")
}

func (receiver *MakeRepository) toSnakeCase(s string) string {
	// Insert underscore before uppercase letters (except the first one)
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(s, "${1}_${2}")

	// Convert to lowercase
	return strings.ToLower(snake)
}

func (receiver *MakeRepository) isPascalCase(s string) bool {
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

func (receiver *MakeRepository) interfaceExists(filePath, interfaceName string) (bool, error) {
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

func (receiver *MakeRepository) addInterfaceToFile(filePath, interfaceName string) error {
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

func (receiver *MakeRepository) generateInterfaceAST(interfaceName string) *ast.GenDecl {
	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Repository")
	methodName := "Create" + baseName

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
											{
												Names: []*ast.Ident{ast.NewIdent(strings.ToLower(baseName))},
												Type: &ast.SelectorExpr{
													X:   ast.NewIdent("models"),
													Sel: ast.NewIdent(baseName),
												},
											},
										},
									},
									Results: &ast.FieldList{
										List: []*ast.Field{
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

func (receiver *MakeRepository) generateInterfaceFileContent(interfaceName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("repository_interface.plain.stub")
	if err != nil {
		return "", err
	}

	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Repository")
	methodName := "Create" + baseName

	// Replace placeholders with actual interface name and method
	content := strings.ReplaceAll(string(stubContent), "DummyRepositoryInterface", interfaceName)
	content = strings.ReplaceAll(content, "CreateDummy", methodName)
	content = strings.ReplaceAll(content, "Dummy", baseName)
	content = strings.ReplaceAll(content, "dummy", strings.ToLower(baseName))

	return content, nil
}

func (receiver *MakeRepository) createModelFile(filePath, modelName string) error {
	content, err := receiver.generateModelContent(modelName)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (receiver *MakeRepository) generateModelContent(modelName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("model.plain.stub")
	if err != nil {
		return "", err
	}

	// Convert model name to snake_case for table name
	tableName := receiver.toSnakeCase(modelName)

	// Replace placeholder with actual model name
	content := strings.ReplaceAll(string(stubContent), "DummyModel", modelName)
	content = strings.ReplaceAll(content, "dummy_model", tableName)

	return content, nil
}

func (receiver *MakeRepository) createRepositoryFile(filePath, repoName, interfaceName string) error {
	// Generate repository content using stub
	structName := receiver.toCamelCase(repoName)
	methodName := receiver.extractMethodName(interfaceName)
	baseName := receiver.extractBaseName(interfaceName)

	content, err := receiver.generateRepositoryContent(structName, repoName, interfaceName, methodName, baseName)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(content), 0644)
}

func (receiver *MakeRepository) toCamelCase(s string) string {
	if s == "" {
		return ""
	}

	// Convert first character to lowercase
	return strings.ToLower(s[:1]) + s[1:]
}

func (receiver *MakeRepository) extractMethodName(interfaceName string) string {
	// Create example method name from interface name
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Repository")
	return "Create" + baseName
}

func (receiver *MakeRepository) extractBaseName(interfaceName string) string {
	// Extract base name for model reference
	baseName := strings.TrimSuffix(interfaceName, "Interface")
	baseName = strings.TrimSuffix(baseName, "Repository")
	return baseName
}

func (receiver *MakeRepository) generateRepositoryContent(structName, repoName, interfaceName, methodName, baseName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("repository.plain.stub")
	if err != nil {
		return "", err
	}

	// Replace placeholders with actual values
	content := strings.ReplaceAll(string(stubContent), "dummyRepository", structName)
	content = strings.ReplaceAll(content, "DummyRepository", repoName)
	content = strings.ReplaceAll(content, "DummyRepositoryInterface", interfaceName)
	content = strings.ReplaceAll(content, "CreateDummy", methodName)
	content = strings.ReplaceAll(content, "Dummy", baseName)
	content = strings.ReplaceAll(content, "dummy", strings.ToLower(baseName))

	return content, nil
}
