package element

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/sunary/sqlize/utils"
)

// Index ...
type Index struct {
	Node
	Typ       ast.IndexKeyType
	IndexType ast.IndexType
	CnsTyp    ast.ConstraintType
	Columns   []string
}

func (i Index) hashValue() string {
	strHash := strings.Join(i.migrationUp(""), ";")
	hash := md5.Sum([]byte(strHash))
	return hex.EncodeToString(hash[:])
}

// hasChangedDefinition reports whether `i` and `old` describe the same index
// name but a different definition, so the migration must replace it instead
// of skipping it.
//
// Trade-off 1 (column order): ordering is part of the comparison via
// utils.SlideStrEqual, which is a positional (not set) comparison. For a
// composite index, MySQL and PostgreSQL both use column order to decide
// which leading-column queries can use the index, so `(a, b)` and `(b, a)`
// are materially different indexes and must trigger a replace. This does
// mean a pure alias/rename of a column keeps its position in Columns (the
// parser stores the resolved column name, not a positional placeholder), so
// renaming a column without reordering it does not by itself look like a
// reorder here.
//
// Trade-off 2 (BTREE/HASH, dialect isolation): IndexType is only ever
// populated by the MySQL parser path (USING btree|hash|...); the PostgreSQL
// and SQLite parser paths never set it, so it stays ast.IndexTypeInvalid
// (String() == "") on both sides for those dialects. Comparing the rendered
// String() means two unset values compare equal (no false positive from
// Postgres-only runs), while an explicit MySQL USING HASH vs the implicit
// default USING BTREE correctly compares unequal. Because the field is only
// ever populated along one dialect's code path, there is no cross-dialect
// pollution: a Postgres index diffed against another Postgres index can
// never pick up a MySQL-only BTREE/HASH distinction.
//
// Trade-off 3 (unique <-> non-unique): toggling Typ (none/unique) is treated
// the same as any other definition change here - it flows into
// MigrateModifyAction, which Index.migrationUp renders as DROP INDEX then
// CREATE INDEX (see the MigrateModifyAction case below), never a dialect
// ALTER-INDEX-in-place statement. This is intentional: rebuilding a unique
// index requires a full table scan to validate uniqueness, and doing that
// as an in-place ALTER would hold a long-lived lock blocking concurrent
// writes in both MySQL and PostgreSQL. DROP+CREATE is the same two
// statements either way, so there is no safe in-place path to special-case,
// and this code does not attempt one - callers that need online index
// rebuilds (e.g. PostgreSQL CREATE INDEX CONCURRENTLY) must do that outside
// generated migrations.
func (i Index) hasChangedDefinition(old Index) bool {
	if i.Typ != old.Typ {
		return true
	}

	if i.IndexType.String() != old.IndexType.String() {
		return true
	}

	if i.CnsTyp != old.CnsTyp {
		return true
	}

	return !utils.SlideStrEqual(i.Columns, old.Columns)
}

func (i Index) migrationUp(tbName string) []string {
	switch i.Action {
	case MigrateNoAction:
		return nil

	case MigrateAddAction:
		if i.CnsTyp == ast.ConstraintPrimaryKey {
			return []string{fmt.Sprintf(sql.CreatePrimaryKeyStm(),
				sql.EscapeSqlName(tbName),
				strings.Join(sql.EscapeSqlNames(i.Columns), ", "))}
		}

		switch i.Typ {
		case ast.IndexKeyTypeNone:
			return []string{fmt.Sprintf(sql.CreateIndexStm(i.IndexType.String()),
				sql.EscapeSqlName(i.Name), sql.EscapeSqlName(tbName),
				strings.Join(sql.EscapeSqlNames(i.Columns), ", "))}

		case ast.IndexKeyTypeUnique:
			return []string{fmt.Sprintf(sql.CreateUniqueIndexStm(i.IndexType.String()),
				sql.EscapeSqlName(i.Name), sql.EscapeSqlName(tbName),
				strings.Join(sql.EscapeSqlNames(i.Columns), ", "))}

		default:
			return nil
		}

	case MigrateRemoveAction:
		if i.CnsTyp == ast.ConstraintPrimaryKey {
			return []string{fmt.Sprintf(sql.DropPrimaryKeyStm(),
				sql.EscapeSqlName(tbName))}
		}

		if sql.IsSqlite() {
			return []string{fmt.Sprintf(sql.DropIndexStm(),
				sql.EscapeSqlName(i.Name))}
		}

		return []string{fmt.Sprintf(sql.DropIndexStm(),
			sql.EscapeSqlName(i.Name),
			sql.EscapeSqlName(tbName))}

	case MigrateModifyAction:
		strRems := make([]string, 2)
		i.Action = MigrateRemoveAction
		strRems[0] = i.migrationUp(tbName)[0]
		i.Action = MigrateAddAction
		strRems[1] = i.migrationUp(tbName)[0]
		return strRems

	case MigrateRenameAction:
		return []string{fmt.Sprintf(sql.AlterTableRenameIndexStm(),
			sql.EscapeSqlName(tbName),
			sql.EscapeSqlName(i.OldName),
			sql.EscapeSqlName(i.Name))}

	default:
		return nil
	}
}

func (i Index) migrationDown(tbName string) []string {
	switch i.Action {
	case MigrateNoAction:
		return nil

	case MigrateAddAction:
		i.Action = MigrateRemoveAction

	case MigrateRemoveAction:
		i.Action = MigrateAddAction

	case MigrateModifyAction:

	case MigrateRenameAction:
		i.Name, i.OldName = i.OldName, i.Name

	default:
		return nil
	}

	return i.migrationUp(tbName)
}
