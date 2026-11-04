// Command check-player-pointer-input rejects Ebitengine pointer APIs in player-game source.
package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	ebitenImportPath    = "github.com/hajimehoshi/ebiten/v2"
	inpututilImportPath = "github.com/hajimehoshi/ebiten/v2/inpututil"
)

type pointerViolation struct {
	position token.Position
	api      string
}

func main() {
	violations, err := findPointerViolations([]string{"cmd/ultimav", "internal/ui"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, violation := range violations {
		fmt.Fprintf(
			os.Stderr,
			"%s: player-game pointer API %s is not allowed\n",
			violation.position,
			violation.api,
		)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

func findPointerViolations(roots []string) ([]pointerViolation, error) {
	fset := token.NewFileSet()
	var violations []pointerViolation
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}

			fileViolations, err := findFilePointerViolations(fset, path)
			if err != nil {
				return err
			}
			violations = append(violations, fileViolations...)

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan player source %q: %w", root, err)
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		left := violations[i].position
		right := violations[j].position
		if left.Filename != right.Filename {
			return left.Filename < right.Filename
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}

		return left.Column < right.Column
	})

	return violations, nil
}

func findFilePointerViolations(fset *token.FileSet, path string) ([]pointerViolation, error) {
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %q: %w", path, err)
	}

	violations, hasPointerImport, err := inspectPointerImports(fset, file, path)
	if err != nil {
		return nil, err
	}
	if !hasPointerImport {
		return violations, nil
	}

	//nolint:exhaustruct_v5 // The checker only needs imported package identifier uses.
	info := types.Info{Uses: make(map[*ast.Ident]types.Object)}
	//nolint:exhaustruct_v5 // Default type-check settings are sufficient for package identifier resolution.
	config := types.Config{
		Importer: importer.Default(),
		Error:    func(_ error) {},
	}
	// Type checking can report unrelated unresolved imports in an individual
	// file; Uses still identifies any successfully resolved pointer imports.
	_, checkErr := config.Check(file.Name.Name, fset, []*ast.File{file}, &info)
	if checkErr != nil && len(info.Uses) == 0 {
		return nil, fmt.Errorf("type-checking %q for pointer input: %w", path, checkErr)
	}

	ast.Inspect(file, func(node ast.Node) bool {
		selector, selectorFound := node.(*ast.SelectorExpr)
		if !selectorFound {
			return true
		}
		identifier, identifierFound := selector.X.(*ast.Ident)
		if !identifierFound {
			return true
		}
		packageName, packageFound := info.Uses[identifier].(*types.PkgName)
		if !packageFound || !isPointerImport(packageName.Imported().Path()) || !isPointerAPI(selector.Sel.Name) {
			return true
		}
		violations = append(violations, pointerViolation{
			position: fset.Position(selector.Pos()),
			api:      identifier.Name + "." + selector.Sel.Name,
		})

		return true
	})

	return violations, nil
}

func inspectPointerImports(
	fset *token.FileSet,
	file *ast.File,
	path string,
) ([]pointerViolation, bool, error) {
	var violations []pointerViolation
	hasPointerImport := false
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, false, fmt.Errorf("parse import in %q: %w", path, err)
		}
		if !isPointerImport(importPath) {
			continue
		}

		importName := filepath.Base(importPath)
		if spec.Name != nil {
			importName = spec.Name.Name
		}
		switch importName {
		case "_":
			continue
		case ".":
			violations = append(violations, pointerViolation{
				position: fset.Position(spec.Pos()),
				api:      "dot import " + importPath,
			})
		default:
			hasPointerImport = true
		}
	}

	return violations, hasPointerImport, nil
}

func isPointerAPI(name string) bool {
	if strings.HasPrefix(name, "MouseButton") {
		return true
	}

	pointerAPIs := []string{
		"AppendJustPressedTouchIDs",
		"AppendJustReleasedTouchIDs",
		"AppendTouchIDs",
		"CursorPosition",
		"IsMouseButtonJustPressed",
		"IsMouseButtonJustReleased",
		"IsMouseButtonPressed",
		"IsTouchJustReleased",
		"JustPressedTouchIDs",
		"MouseButtonPressDuration",
		"TouchID",
		"TouchIDs",
		"TouchPosition",
		"TouchPositionInPreviousTick",
		"TouchPressDuration",
		"Wheel",
	}

	return slices.Contains(pointerAPIs, name)
}

func isPointerImport(importPath string) bool {
	return importPath == ebitenImportPath || importPath == inpututilImportPath
}
