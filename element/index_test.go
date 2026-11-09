package element

import (
	"strings"
	"testing"

	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/sunary/sqlize/sqltemplates"
)

// diffIndexUp builds an "old" table with a single index, a "new" table with
// the same index name but a different definition, diffs them and returns the
// generated migration-up SQL (joined, one statement per line).
func diffIndexUp(t *testing.T, oldIdx, newIdx Index) string {
	t.Helper()

	NewMigration(sqltemplates.MysqlDialect, false, false)

	oldIdx.Action = MigrateAddAction
	newIdx.Action = MigrateAddAction

	oldTable := NewTableWithAction("tbl", MigrateNoAction)
	oldTable.AddIndex(oldIdx)

	newTable := NewTableWithAction("tbl", MigrateNoAction)
	newTable.AddIndex(newIdx)

	newTable.Diff(*oldTable)

	strSqls := newTable.MigrationIndexUp(map[string]struct{}{})
	return strings.Join(strSqls, "\n")
}

func TestIndexDiff_BtreeToHash(t *testing.T) {
	oldIdx := Index{
		Node:      Node{Name: "idx_name"},
		Typ:       ast.IndexKeyTypeNone,
		IndexType: ast.IndexTypeBtree,
		Columns:   []string{"name"},
	}
	newIdx := Index{
		Node:      Node{Name: "idx_name"},
		Typ:       ast.IndexKeyTypeNone,
		IndexType: ast.IndexTypeHash,
		Columns:   []string{"name"},
	}

	got := diffIndexUp(t, oldIdx, newIdx)
	t.Logf("generated SQL:\n%s", got)

	if !strings.Contains(got, "DROP INDEX `idx_name` ON `tbl`;") {
		t.Fatalf("expected DROP INDEX for changed index type, got:\n%s", got)
	}
	if !strings.Contains(got, "CREATE INDEX `idx_name` ON `tbl`(`name`) USING HASH;") {
		t.Fatalf("expected CREATE INDEX ... USING HASH, got:\n%s", got)
	}
}

func TestIndexDiff_UniqueToNonUnique(t *testing.T) {
	oldIdx := Index{
		Node:    Node{Name: "idx_email"},
		Typ:     ast.IndexKeyTypeUnique,
		Columns: []string{"email"},
	}
	newIdx := Index{
		Node:    Node{Name: "idx_email"},
		Typ:     ast.IndexKeyTypeNone,
		Columns: []string{"email"},
	}

	got := diffIndexUp(t, oldIdx, newIdx)
	t.Logf("generated SQL:\n%s", got)

	if !strings.Contains(got, "DROP INDEX `idx_email` ON `tbl`;") {
		t.Fatalf("expected DROP INDEX for unique -> non-unique change, got:\n%s", got)
	}
	if !strings.Contains(got, "CREATE INDEX `idx_email` ON `tbl`(`email`);") {
		t.Fatalf("expected CREATE INDEX (non-unique), got:\n%s", got)
	}
	if strings.Contains(got, "CREATE UNIQUE INDEX") {
		t.Fatalf("new definition is non-unique, must not emit CREATE UNIQUE INDEX, got:\n%s", got)
	}
}

func TestIndexDiff_ColumnOrderChanged(t *testing.T) {
	oldIdx := Index{
		Node:    Node{Name: "idx_name_age"},
		Typ:     ast.IndexKeyTypeNone,
		Columns: []string{"name", "age"},
	}
	newIdx := Index{
		Node:    Node{Name: "idx_name_age"},
		Typ:     ast.IndexKeyTypeNone,
		Columns: []string{"age", "name"},
	}

	got := diffIndexUp(t, oldIdx, newIdx)
	t.Logf("generated SQL:\n%s", got)

	if !strings.Contains(got, "DROP INDEX `idx_name_age` ON `tbl`;") {
		t.Fatalf("expected DROP INDEX for reordered composite index, got:\n%s", got)
	}
	if !strings.Contains(got, "CREATE INDEX `idx_name_age` ON `tbl`(`age`, `name`);") {
		t.Fatalf("expected CREATE INDEX with new column order (age, name), got:\n%s", got)
	}
}

func TestIndexDiff_SameDefinition_NoChange(t *testing.T) {
	oldIdx := Index{
		Node:      Node{Name: "idx_name"},
		Typ:       ast.IndexKeyTypeNone,
		IndexType: ast.IndexTypeBtree,
		Columns:   []string{"name"},
	}
	newIdx := Index{
		Node:      Node{Name: "idx_name"},
		Typ:       ast.IndexKeyTypeNone,
		IndexType: ast.IndexTypeBtree,
		Columns:   []string{"name"},
	}

	got := diffIndexUp(t, oldIdx, newIdx)
	t.Logf("generated SQL (expected empty):\n%s", got)

	if got != "" {
		t.Fatalf("identical index definitions must not generate any migration, got:\n%s", got)
	}
}
