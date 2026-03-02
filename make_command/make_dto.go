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

type MakeDto struct{}

// Signature The name and signature of the console command.
func (receiver *MakeDto) Signature() string {
	return "make:dto"
}

// Description The console command description.
func (receiver *MakeDto) Description() string {
	return "Create a new dto class"
}

// Extend The console command extend.
func (receiver *MakeDto) Extend() command.Extend {
	return command.Extend{
		Category: "make",
	}
}

// Handle Execute the console command.
func (receiver *MakeDto) Handle(ctx console.Context) error {
	dtoName := ctx.Argument(0)
	if dtoName == "" {
		facades.Log().Error("DTO name is required")
		return fmt.Errorf("DTO name is required")
	}

	// Validate PascalCase
	if !receiver.isPascalCase(dtoName) {
		facades.Log().Error("DTO name must be in PascalCase (e.g., SyncBakiDebetItem)")
		return fmt.Errorf("DTO name must be in PascalCase")
	}

	// Convert to snake_case for filename
	fileName := receiver.toSnakeCase(dtoName)

	// Create models directory if it doesn't exist
	dtoDir := "app/models/dtos"
	if err := os.MkdirAll(dtoDir, 0755); err != nil {
		facades.Log().Error("Failed to create DTO directory: " + err.Error())
		return err
	}

	// Full file path
	filePath := filepath.Join(dtoDir, fileName+".go")

	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		facades.Log().Error("DTO already exists: " + filePath)
		return fmt.Errorf("DTO already exists: %s", filePath)
	}

	// Generate DTO content
	content, err := receiver.generateDtoContent(dtoName, fileName)
	if err != nil {
		facades.Log().Error(fmt.Sprintf("Failed to generate DTO, err -> %s", err))
	}

	// Write file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		facades.Log().Error("Failed to create DTO file: " + err.Error())
		return err
	}

	facades.Log().Info("DTO created successfully: " + filePath)
	return nil
}

func (receiver *MakeDto) isPascalCase(s string) bool {
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

func (receiver *MakeDto) toSnakeCase(s string) string {
	// Insert underscore before uppercase letters (except the first one)
	re := regexp.MustCompile("([a-z0-9])([A-Z])")
	snake := re.ReplaceAllString(s, "${1}_${2}")

	// Convert to lowercase
	return strings.ToLower(snake)
}

func (receiver *MakeDto) generateDtoContent(dtoName, tableName string) (string, error) {
	// Read stub file from embedded filesystem
	stubContent, err := stubs.StubsFS.ReadFile("dto.plain.stub")
	if err != nil {
		return "", err
	}

	// Replace placeholder with actual model name
	content := strings.ReplaceAll(string(stubContent), "DummyDto", dtoName)

	return content, nil
}
