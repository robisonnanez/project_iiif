// Command check-comments verifica la documentación mínima de funciones de producción.
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var frontendFunction = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+|^const\s+[A-Za-z0-9_]+\s*=\s*(?:async\s*)?\(.*\)\s*=>`)

// main recorre backend y frontend y falla si una función nombrada carece de comentario de propósito.
func main() {
	root := "."
	if filepath.Base(mustWorkingDirectory()) == "backend" {
		root = ".."
	}
	errors := append(checkGo(filepath.Join(root, "backend")), checkFrontend(filepath.Join(root, "backend", "frontend", "src"))...)
	if len(errors) == 0 {
		fmt.Println("Comentarios de funciones: OK")
		return
	}
	for _, problem := range errors {
		fmt.Fprintln(os.Stderr, problem)
	}
	os.Exit(1)
}

// mustWorkingDirectory obtiene el directorio actual o detiene la verificación con un error claro.
func mustWorkingDirectory() string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return workingDirectory
}

// checkGo usa el AST para exigir documentación en funciones Go que forman parte del binario.
func checkGo(root string) []string {
	var problems []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			problems = append(problems, walkErr.Error())
			return nil
		}
		if entry.IsDir() && (entry.Name() == "docs" || entry.Name() == "vendor") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", path, err))
			return nil
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && (function.Doc == nil || !strings.Contains("\n"+strings.TrimSpace(function.Doc.Text()), "\n"+function.Name.Name+" ")) {
				position := fileSet.Position(function.Pos())
				problems = append(problems, fmt.Sprintf("%s:%d: falta comentario de propósito para %s", path, position.Line, function.Name.Name))
			}
		}
		return nil
	})
	return problems
}

// checkFrontend comprueba componentes y funciones nombradas, omitiendo pruebas y callbacks anónimos.
func checkFrontend(root string) []string {
	var problems []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			problems = append(problems, walkErr.Error())
			return nil
		}
		if entry.IsDir() || (!strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx")) || strings.Contains(path, ".test.") {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			problems = append(problems, err.Error())
			return nil
		}
		defer file.Close()
		scanner, lineNumber, previousNonEmpty := bufio.NewScanner(file), 0, ""
		for scanner.Scan() {
			lineNumber++
			line := strings.TrimSpace(scanner.Text())
			if frontendFunction.MatchString(line) && !strings.HasPrefix(previousNonEmpty, "//") {
				problems = append(problems, fmt.Sprintf("%s:%d: falta comentario para función frontend", path, lineNumber))
			}
			if line != "" {
				previousNonEmpty = line
			}
		}
		return nil
	})
	return problems
}
