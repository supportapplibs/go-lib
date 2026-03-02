package make_command

import (
	"fmt"
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"
	"github.com/goravel/framework/facades"
	"github.com/supportapplibs/go-lib/stubs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

type MakeModel struct{}

// Signature The name and signature of the console command.
func (receiver *MakeModel) Signature() string {
	return "make:custom-model"
}

// Description The console command description.
func (receiver *MakeModel) Description() string {
	return "Create a new model class"
}

// Extend The console command extend.
func (receiver *MakeModel) Extend() command.Extend {
	return command.Extend{
		Category: "make",
	}
}

// Handle Execute the console command.
func (receiver *MakeModel) Handle(ctx console.Context) error {
	modelName := ctx.Argument(0)
	if modelName == "" {
		facades.Log().Error("Model name is required")
		return fmt.Errorf("model name is required")
	}

	// Validate PascalCase
	if !receiver.isPascalCase(modelName) {
		facades.Log().Error("Model name must be in PascalCase (e.g., UserProfile)")
		return fmt.Errorf("model name must be in PascalCase")
	}

	// Convert to snake_case for filename
	fileName := receiver.toSnakeCase(modelName)

	// Create models directory if it doesn't exist
	modelsDir := "app/models"
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		facades.Log().Error("Failed to create models directory: " + err.Error())
		return err
	}

	// Create stubs directory if it doesn't exist
	stubsDir := "stubs"
	if err := os.MkdirAll(stubsDir, 0755); err != nil {
		facades.Log().Error("Failed to create stubs directory: " + err.Error())
		return err
	}

	// Full file path
	filePath := filepath.Join(modelsDir, fileName+".go")

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		facades.Log().Error("Model already exists: " + filePath)
		return fmt.Errorf("model already exists: %s", filePath)
	}

	// Generate model content
	content, err := receiver.generateModelContent(modelName, fileName)
	if err != nil {
		facades.Log().Error(fmt.Sprintf("Failed to generate model, err -> %s", err))
		return err
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		facades.Log().Error("Failed to create model file: " + err.Error())
		return err
	}

	facades.Log().Info("Model created successfully: " + filePath)
	return nil
}

func (receiver *MakeModel) isPascalCase(s string) bool {
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

func (receiver *MakeModel) toSnakeCase(s string) string {
	// Insert underscore before uppercase letters (except the first one)
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(s, "${1}_${2}")

	// Convert to lowercase
	return strings.ToLower(snake)
}

func (receiver *MakeModel) generateModelContent(modelName, tableName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("model.plain.stub")
	if err != nil {
		return "", err
	}

	// Replace placeholder with actual model name
	content := strings.ReplaceAll(string(stubContent), "DummyModel", modelName)
	content = strings.ReplaceAll(content, "dummy_model", tableName)

	return content, nil
}
