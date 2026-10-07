package core

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCoreModuleImportsNoFrameworkOrContrib(t *testing.T) {
	root := filepath.Join("..")
	forbidden := []string{"github.com/gofiber/", "gorm.io/", "github.com/MagicRodri/go-polyadmin/contrib/"}
	var offending []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (name == "contrib" || name == "examples" || name == "browsertests" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			for _, f := range forbidden {
				if strings.HasPrefix(p, f) {
					offending = append(offending, path+": "+p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offending) > 0 {
		t.Fatalf("core module imports a framework or contrib package:\n%s", strings.Join(offending, "\n"))
	}
}
