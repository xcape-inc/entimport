package entimport

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"ariga.io/atlas/sql/postgres"
	"ariga.io/atlas/sql/schema"

	"entgo.io/contrib/schemast"
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// Postgres implements SchemaImporter for PostgreSQL databases.
type Postgres struct {
	*ImportOptions
}

// NewPostgreSQL - returns a new *Postgres.
func NewPostgreSQL(i *ImportOptions) (SchemaImporter, error) {
	return &Postgres{
		ImportOptions: i,
	}, nil
}

// SchemaMutations implements SchemaImporter.
func (p *Postgres) SchemaMutations(ctx context.Context) ([]schemast.Mutator, error) {
	inspectOptions := &schema.InspectOptions{
		Tables: p.tables,
	}
	s, err := p.driver.InspectSchema(ctx, p.driver.SchemaName, inspectOptions)
	if err != nil {
		return nil, err
	}
	tables := s.Tables
	if p.excludedTables != nil {
		tables = nil
		excludedTableNames := make(map[string]bool)
		for _, t := range p.excludedTables {
			excludedTableNames[t] = true
		}
		// filter out tables that are in excludedTables:
		for _, t := range s.Tables {
			if !excludedTableNames[t.Name] {
				tables = append(tables, t)
			}
		}
	}
	return schemaMutations(p.field, tables)
}

type Strings []string

func (s *Strings) Scan(v any) (err error) {
	switch v := v.(type) {
	case nil:
	case []byte:
		err = s.scan(string(v))
	case string:
		err = s.scan(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

func (s *Strings) scan(v string) error {
	if v == "" {
		return nil
	}
	if l := len(v); l < 2 || v[0] != '{' && v[l-1] != '}' {
		return fmt.Errorf("unexpected array format %q", v)
	}
	*s = strings.Split(v[1:len(v)-1], ",")
	return nil
}

func (s Strings) Value() (driver.Value, error) {
	return "{" + strings.Join(s, ",") + "}", nil
}

func (p *Postgres) field(column *schema.Column) (f ent.Field, err error) {
	name := column.Name
	var textType schema.Type = &schema.StringType{T: "text", Size: 0, Attrs: []schema.Attr{}}
	typeX := reflect.TypeOf(textType)
	typeY := reflect.TypeOf(column.Type.Type)

	switch typ := column.Type.Type.(type) {
	case *schema.BinaryType:
		f = field.Bytes(name)
	case *schema.BoolType:
		f = field.Bool(name)
	case *schema.DecimalType:
		f = field.Float(name)
	case *schema.EnumType:
		f = field.Enum(name).Values(typ.Values...)
	case *schema.FloatType:
		f = p.convertFloat(typ, name)
	case *schema.IntegerType:
		f = p.convertInteger(typ, name)
	case *schema.JSONType:
		f = field.JSON(name, json.RawMessage{})
	case *schema.StringType:
		f = field.String(name)
	case *schema.TimeType:
		f = field.Time(name)
	case *postgres.SerialType:
		f = p.convertSerial(typ, name)
	case *postgres.UUIDType:
		f = field.UUID(name, uuid.New())
	/*case "&{%!q(*schema.StringType=&{text 0 []}) \"text[]\"}":*/
	// f = field.UUID(name, uuid.New())
	case *postgres.ArrayType:
		arrayTypeInstance := column.Type.Type.(*postgres.ArrayType)

		switch arrtyp := arrayTypeInstance.Underlying().(type) {
		/*case *schema.BinaryType:
			f = field.Bytes(name)
		case *schema.BoolType:
			f = field.Bool(name)
		case *schema.DecimalType:
			f = field.Float(name)
		case *schema.EnumType:
			f = field.Enum(name).Values(typ.Values...)
		case *schema.FloatType:
			f = p.convertFloat(typ, name)
		case *schema.IntegerType:
			f = p.convertInteger(typ, name)
		case *schema.JSONType:
			f = field.JSON(name, json.RawMessage{})*/
		case *schema.StringType:
			f = field.Other(name, Strings{}).
				SchemaType(map[string]string{
					dialect.Postgres: arrayTypeInstance.T,
					dialect.SQLite:   "json",
					dialect.MySQL:    "blob",
				})
		/*case *schema.TimeType:
			f = field.Time(name)
		case *postgres.SerialType:
			f = p.convertSerial(typ, name)
		case *postgres.UUIDType:
			f = field.UUID(name, uuid.New())*/
		default:
			fmt.Println("textType is of type: ", typeX)
			fmt.Println("column.Type.Type is of type: ", typeY)
			fmt.Println("data is of type int with value: ", typ)
			fmt.Println("item is of type int with value: ", arrtyp)
			return nil, fmt.Errorf("entimport: unsupported array item type %q for column %v", arrtyp, column.Name)
		}
	default:
		fmt.Println("textType is of type: ", typeX)
		fmt.Println("column.Type.Type is of type: ", typeY)
		fmt.Println("data is of type int with value: ", typ)
		return nil, fmt.Errorf("entimport: unsupported type %q for column %v", typ, column.Name)
	}
	applyColumnAttributes(f, column)
	return f, err
}

// decimal, numeric - user-specified precision, exact up to 131072 digits before the decimal point;
// up to 16383 digits after the decimal point.
// real - 4 bytes variable-precision, inexact 6 decimal digits precision.
// double -	8 bytes	variable-precision, inexact	15 decimal digits precision.
func (p *Postgres) convertFloat(typ *schema.FloatType, name string) (f ent.Field) {
	if typ.T == postgres.TypeReal {
		return field.Float32(name)
	}
	return field.Float(name)
}

func (p *Postgres) convertInteger(typ *schema.IntegerType, name string) (f ent.Field) {
	switch typ.T {
	// smallint - 2 bytes small-range integer -32768 to +32767.
	case "smallint":
		f = field.Int16(name)
	// integer - 4 bytes typical choice for integer	-2147483648 to +2147483647.
	case "integer":
		f = field.Int32(name)
	// bigint - 8 bytes large-range integer	-9223372036854775808 to 9223372036854775807.
	case "bigint":
		// Int64 is not used on purpose.
		f = field.Int(name)
	}
	return f
}

// smallserial- 2 bytes - small autoincrementing integer 1 to 32767
// serial - 4 bytes autoincrementing integer 1 to 2147483647
// bigserial - 8 bytes large autoincrementing integer	1 to 9223372036854775807
func (p *Postgres) convertSerial(typ *postgres.SerialType, name string) ent.Field {
	return field.Uint(name).
		SchemaType(map[string]string{
			dialect.Postgres: typ.T, // Override Postgres.
		})
}
