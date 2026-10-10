package gorm

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

// DingoDB speaks the MySQL protocol, but its schema representation and DDL
// differ. Select the adapter from the server version, not the shared "mysql"
// configuration value. Ordinary MySQL keeps the upstream migrator unchanged.
type mysqlCompatibilityDialector struct{ *mysql.Dialector }

func mysqlDialector(dsn string) gorm.Dialector {
	return mysqlCompatibilityDialector{mysql.Open(dsn).(*mysql.Dialector)}
}

func (d mysqlCompatibilityDialector) isDingo() bool {
	return strings.Contains(strings.ToLower(d.ServerVersion), "dingodb")
}

func (d mysqlCompatibilityDialector) DataTypeOf(field *schema.Field) string {
	typ := d.Dialector.DataTypeOf(field)
	if !d.isDingo() {
		return typ
	}
	// Match the types DingoDB persists, including tables created by the old
	// MySQL dialector. Do not change the shared model tags or MySQL columns.
	if strings.HasPrefix(typ, "datetime") {
		return "timestamp" + strings.TrimPrefix(typ, "datetime")
	}
	switch typ {
	case "text":
		return "varchar(65535)"
	case "longtext":
		return "varchar(65536)"
	}
	return typ
}

func (d mysqlCompatibilityDialector) Migrator(db *gorm.DB) gorm.Migrator {
	m := d.Dialector.Migrator(db).(mysql.Migrator)
	if !d.isDingo() {
		return m
	}
	m.Migrator.Config.Dialector = d
	return dingoMigrator{Migrator: m}
}

type dingoMigrator struct{ mysql.Migrator }

func (m dingoMigrator) ColumnTypes(value interface{}) ([]gorm.ColumnType, error) {
	columns, err := m.Migrator.ColumnTypes(value)
	if err != nil {
		return nil, err
	}
	for i, column := range columns {
		if c, ok := column.(migrator.ColumnType); ok && strings.EqualFold(c.DatabaseTypeName(), "timestamp") {
			// DingoDB exposes fractional-second precision as character length;
			// datetime_precision is NULL and the wire protocol reports zero.
			// Normalize metadata, so a real precision change is still detected.
			if !c.DecimalSizeValue.Valid && c.LengthValue.Valid && c.LengthValue.Int64 >= 0 && c.LengthValue.Int64 <= 6 {
				c.DecimalSizeValue = c.LengthValue
				c.ScaleValue = sql.NullInt64{Int64: 0, Valid: true}
				columns[i] = c
			}
		}
	}
	return columns, nil
}

func (m dingoMigrator) GetIndexes(value interface{}) ([]gorm.Index, error) {
	var indexes []gorm.Index
	err := m.RunWithValue(value, func(stmt *gorm.Statement) error {
		// information_schema.statistics uses the index's own table name for
		// secondary indexes in DingoDB. SHOW INDEX retains the owning table.
		var rows []struct {
			KeyName    string `gorm:"column:key_name"`
			ColumnName string `gorm:"column:column_name"`
			NonUnique  int    `gorm:"column:non_unique"`
			Seq        int    `gorm:"column:seq_in_index"`
		}
		if err := m.DB.Raw("SHOW INDEX FROM ?", m.CurrentTable(stmt)).Scan(&rows).Error; err != nil {
			return err
		}
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].KeyName == rows[j].KeyName {
				return rows[i].Seq < rows[j].Seq
			}
			return rows[i].KeyName < rows[j].KeyName
		})
		byName := map[string]*migrator.Index{}
		for _, row := range rows {
			idx := byName[row.KeyName]
			if idx == nil {
				idx = &migrator.Index{TableName: stmt.Table, NameValue: row.KeyName,
					PrimaryKeyValue: sql.NullBool{Bool: row.KeyName == "PRIMARY", Valid: true},
					UniqueValue:     sql.NullBool{Bool: row.NonUnique == 0, Valid: true}}
				byName[row.KeyName] = idx
				indexes = append(indexes, idx)
			}
			idx.ColumnList = append(idx.ColumnList, row.ColumnName)
		}
		return nil
	})
	return indexes, err
}

func (m dingoMigrator) HasIndex(value interface{}, name string) bool {
	_ = m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}
		return nil
	})
	indexes, err := m.GetIndexes(value)
	if err != nil {
		m.DB.AddError(err)
		return false
	}
	for _, idx := range indexes {
		if strings.EqualFold(idx.Name(), name) {
			return true
		}
	}
	return false
}

func (m dingoMigrator) CreateIndex(value interface{}, name string) error {
	// HasIndex's GORM interface cannot return an error. Recheck metadata before
	// issuing DDL so a failed inspection is not treated as a missing index.
	if _, err := m.GetIndexes(value); err != nil {
		return fmt.Errorf("inspect DingoDB indexes before creating %q: %w", name, err)
	}
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema == nil {
			return fmt.Errorf("DingoDB index %q: missing model schema", name)
		}
		idx := stmt.Schema.LookIndex(name)
		if idx == nil {
			return fmt.Errorf("DingoDB index %q: not found in model", name)
		}
		if idx.Type != "" || idx.Option != "" || idx.Comment != "" || (idx.Class != "" && idx.Class != "UNIQUE") {
			return fmt.Errorf("DingoDB index %q: unsupported index options", name)
		}
		return m.DB.Exec("ALTER TABLE ? ADD "+idx.Class+" INDEX ? (?)",
			m.CurrentTable(stmt), clause.Column{Name: idx.Name}, m.BuildIndexOptions(idx.Fields, stmt)).Error
	})
}

func (m dingoMigrator) AlterColumn(value interface{}, name string) error {
	if err := m.Migrator.AlterColumn(value, name); err != nil {
		return fmt.Errorf("DingoDB cannot migrate column %q; review the requested schema change and server DDL support: %w", name, err)
	}
	return nil
}

var dingoBareIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (m dingoMigrator) DropIndex(value interface{}, name string) error {
	return m.RunWithValue(value, func(stmt *gorm.Statement) error {
		if stmt.Schema != nil {
			if idx := stmt.Schema.LookIndex(name); idx != nil {
				name = idx.Name
			}
		}
		// DingoDB includes backticks in DROP INDEX's lookup key. Only emit a
		// bare identifier after strict validation; never interpolate arbitrary SQL.
		if !dingoBareIdentifier.MatchString(name) {
			return fmt.Errorf("DingoDB DROP INDEX requires a simple identifier: %q", name)
		}
		return m.DB.Exec("ALTER TABLE ? DROP INDEX "+name, m.CurrentTable(stmt)).Error
	})
}
