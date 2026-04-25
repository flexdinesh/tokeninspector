package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestSchemaContract(t *testing.T) {
	_, testFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(testFile)

	schemaSQLPath := filepath.Join(testDir, "../../../", "schema", "schema.sql")
	schemaGoPath := filepath.Join(testDir, "schema.go")

	sqlBytes, err := os.ReadFile(schemaSQLPath)
	if err != nil {
		t.Fatalf("read schema.sql: %v", err)
	}

	sqlTables, sqlVersion := parseSchemaSQL(string(sqlBytes))

	goTables, goCols, err := parseSchemaGo(schemaGoPath)
	if err != nil {
		t.Fatalf("parse schema.go: %v", err)
	}

	// Verify (a): all Go Table* constants have matching tables
	for name, val := range goTables {
		if _, ok := sqlTables[val]; !ok {
			t.Errorf("Go constant %s = %q has no matching table in schema.sql", name, val)
		}
	}

	// Build set of all schema columns
	allSQLCols := make(map[string]bool)
	for _, cols := range sqlTables {
		for _, c := range cols {
			allSQLCols[c] = true
		}
	}

	// Verify (b): all Go Col* constants have matching column in at least one table
	for name, val := range goCols {
		if !allSQLCols[val] {
			t.Errorf("Go constant %s = %q has no matching column in any table in schema.sql", name, val)
		}
	}

	// Verify (c): for each table, all its column names have at least one matching Go constant
	colToGoConsts := make(map[string][]string)
	for name, val := range goCols {
		colToGoConsts[val] = append(colToGoConsts[val], name)
	}

	for tableName, cols := range sqlTables {
		for _, col := range cols {
			if len(colToGoConsts[col]) == 0 {
				t.Errorf("schema column %q in table %q has no matching Go Col* constant", col, tableName)
			}
		}
	}

	// Verify (d): SupportedSchemaVersion matches PRAGMA user_version
	if SupportedSchemaVersion != sqlVersion {
		t.Errorf("SupportedSchemaVersion = %d, but schema.sql PRAGMA user_version = %d", SupportedSchemaVersion, sqlVersion)
	}
}

func parseSchemaSQL(content string) (map[string][]string, int) {
	tables := make(map[string][]string)
	lines := strings.Split(content, "\n")

	var currentTable string
	parenDepth := 0
	var schemaVersion int

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		// PRAGMA user_version
		if strings.HasPrefix(upper, "PRAGMA USER_VERSION") {
			for _, f := range strings.Fields(trimmed) {
				f = strings.TrimSuffix(strings.TrimSuffix(f, ";"), ",")
				if v, err := strconv.Atoi(f); err == nil {
					schemaVersion = v
					break
				}
			}
			continue
		}

		// CREATE TABLE
		if strings.Contains(upper, "CREATE TABLE") {
			var afterTable string
			if idx := strings.Index(upper, "IF NOT EXISTS"); idx != -1 {
				afterTable = trimmed[idx+len("IF NOT EXISTS"):]
			} else if idx := strings.Index(upper, "TABLE"); idx != -1 {
				afterTable = trimmed[idx+len("TABLE"):]
			}
			afterTable = strings.TrimSpace(afterTable)

			parenIdx := strings.Index(afterTable, "(")
			if parenIdx == -1 {
				currentTable = afterTable
				parenDepth = 0
				continue
			}

			currentTable = strings.TrimSpace(afterTable[:parenIdx])
			parenDepth = 1
			tables[currentTable] = []string{}

			rest := strings.TrimSpace(afterTable[parenIdx+1:])
			if rest != "" {
				parenDepth += strings.Count(rest, "(") - strings.Count(rest, ")")
				if parenDepth > 0 {
					if col := extractColumn(rest); col != "" {
						tables[currentTable] = append(tables[currentTable], col)
					}
				}
			}
			continue
		}

		if currentTable == "" {
			continue
		}

		parenDepth += strings.Count(line, "(") - strings.Count(line, ")")

		if parenDepth <= 0 {
			currentTable = ""
			parenDepth = 0
			continue
		}

		if col := extractColumn(trimmed); col != "" {
			tables[currentTable] = append(tables[currentTable], col)
		}
	}

	return tables, schemaVersion
}

func extractColumn(line string) string {
	if line == "" || strings.HasPrefix(line, "--") {
		return ""
	}

	line = strings.TrimSuffix(line, ",")
	line = strings.TrimSuffix(line, ";")
	line = strings.TrimSpace(line)

	if line == "" {
		return ""
	}

	if strings.HasPrefix(line, ")") {
		return ""
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}

	first := fields[0]
	// Strip any trailing ( so UNIQUE(...) is recognised as a constraint keyword
	checkToken := strings.ToUpper(strings.SplitN(first, "(", 2)[0])
	switch checkToken {
	case "PRIMARY", "UNIQUE", "CHECK", "FOREIGN", "CONSTRAINT":
		return ""
	}

	return first
}

func parseSchemaGo(path string) (map[string]string, map[string]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, err
	}

	tables := make(map[string]string)
	cols := make(map[string]string)

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valSpec.Names) == 0 || len(valSpec.Values) == 0 {
				continue
			}
			name := valSpec.Names[0].Name
			lit, ok := valSpec.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			strVal, err := strconv.Unquote(lit.Value)
			if err != nil {
				continue
			}
			if strings.HasPrefix(name, "Table") {
				tables[name] = strVal
			} else if strings.HasPrefix(name, "Col") {
				cols[name] = strVal
			}
		}
	}

	return tables, cols, nil
}
