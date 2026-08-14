package storage

import (
	"reflect"
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	input := "CREATE TABLE first_table (id INT);\n\nCREATE TABLE second_table (id INT);\n"
	want := []string{
		"CREATE TABLE first_table (id INT)",
		"CREATE TABLE second_table (id INT)",
	}
	if got := splitSQLStatements(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSQLStatements() = %#v, want %#v", got, want)
	}
}
