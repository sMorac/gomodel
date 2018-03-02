package model

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/jackc/pgx"
	"github.com/jmoiron/sqlx/reflectx"
	"reflect"
	"strconv"
	"time"
	"unicode"
)

type Model interface {
	GetId() int
	Load(store *ModelStore, id int) error
	Save(store *ModelStore) error
	Delete(store *ModelStore) error
	FillMeta(id int, created_at time.Time, updated_at time.Time)
}
type ModelStore struct {
	Pool      *pgx.ConnPool
	TableName string
}

func NewStore(pool *pgx.ConnPool, tableName string) *ModelStore {
	store := &ModelStore{}
	//	sqlx.NameMapper = ToSnake
	store.Pool = pool
	store.TableName = tableName
	return store
}

func makeStringForExec(model Model) (string, string, []interface{}) {
	var fieldBuffer, valueBuffer bytes.Buffer
	v := reflect.Indirect(reflect.ValueOf(model))
	t := v.Type()
	numField := v.NumField()
	values := make([]interface{}, numField-3)
	fmt.Println(numField)
	idx := 0
	for posField := 0; posField < numField; posField++ {
		switch t.Field(posField).Name {
		case "Id", "UpdatedAt", "CreatedAt":
			continue
		}
		idx++
		fieldName := ToSnake(t.Field(posField).Name)
		fieldBuffer.WriteString(fieldName + ", ")
		valueBuffer.WriteString("$" + strconv.Itoa(idx) + ", ")
		fmt.Println("norm")
		values[idx-1] = v.Field(posField).Interface()
	}
	field_str := fieldBuffer.String()
	value_str := valueBuffer.String()
	return field_str[:len(field_str)-2], value_str[:len(value_str)-2], values
}

func (store *ModelStore) CreateModel(model Model) error {
	field_str, value_str, value_map := makeStringForExec(model)
	var buffer bytes.Buffer
	buffer.WriteString("INSERT INTO ")
	buffer.WriteString(store.TableName)
	buffer.WriteString(" (")
	buffer.WriteString(field_str)
	buffer.WriteString(") VALUES (")
	buffer.WriteString(value_str)
	buffer.WriteString(") RETURNING id, created_at, updated_at")
	query_string := buffer.String()
	rows, err := store.Pool.Query(query_string, value_map...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		var created_at, updated_at time.Time
		rows.Scan(&id, &created_at, &updated_at)
		model.FillMeta(id, created_at, updated_at)
	}
	return nil
}

func (store *ModelStore) LoadModel(model Model, id int) error {
	rows, err := store.Pool.Query("SELECT * FROM "+store.TableName+" WHERE id = $1", id)
	rows.Next()
	err = StructScan(rows, model)
	switch err {
	case pgx.ErrNoRows:
		fmt.Println("No rows returned")
	}
	return err
}

func (store *ModelStore) LoadModels(idList []int, modelSlice []Model) []Model {

	id_list_str := ""
	for _, id := range idList {
		id_list_str += strconv.Itoa(id) + ","
	}
	if len(id_list_str) == 0 {
		return nil
	}
	rows, err := store.Pool.Query("SELECT * FROM " + store.TableName + " WHERE id IN (" + id_list_str[:len(id_list_str)-1] + ") ORDER BY id")
	switch err {
	case nil:
	case pgx.ErrNoRows:
		fmt.Println("No rows returned")
		return nil
	default:
		panic(err)
	}
	index := 0
	for rows.Next() {
		StructScan(rows, modelSlice[index])
		index++
	}
	return modelSlice
}

func StructScan(rows *pgx.Rows, dest interface{}) error {
	started := false
	/* fmt.Println("//// values ///")
	fmt.Println(rows.Values())
	fmt.Println("//// field descriptions ///")
	fmt.Println(rows.FieldDescriptions()) */
	var values []interface{}
	var fields [][]int
	v := reflect.ValueOf(dest)

	if v.Kind() != reflect.Ptr {
		return errors.New("must be a pointer, not a value")
	}

	v = reflect.Indirect(v)
	if !started {
		fd := rows.FieldDescriptions()
		columns := make([]string, len(fd))
		for i, f := range fd {
			columns[i] = f.Name
		}
		m := reflectx.NewMapperFunc("db", ToSnake)
		fields = m.TraversalsByName(v.Type(), columns)
		for i, t := range fields {
			if len(t) == 0 {
				return errors.New("missing destination name " + columns[i])
			}
		}

		values = make([]interface{}, len(columns))
		started = true
	}

	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return errors.New("argument not a struct")
	}

	for i, traversal := range fields {
		if len(traversal) == 0 {
			values[i] = new(interface{})

			continue
		}
		f := reflectx.FieldByIndexes(v, traversal)
		// fmt.Println(f, v, traversal)
		values[i] = f.Addr().Interface()
	}

	// scan into the struct field pointers and append to our results
	err := rows.Scan(values...)
	if err != nil {
		return err
	}
	return nil
}

func (store *ModelStore) UpdateModel(model Model) error {
	if model.GetId() == 0 {
		return errors.New("No id stored, can't find record in DB")
	}
	field_str, value_str, value_map := makeStringForExec(model)
	var buffer bytes.Buffer
	buffer.WriteString("UPDATE ")
	buffer.WriteString(store.TableName)
	buffer.WriteString(" SET (")
	buffer.WriteString(field_str)
	buffer.WriteString(") = (")
	buffer.WriteString(value_str)
	buffer.WriteString(") WHERE id = " + strconv.Itoa(model.GetId()))
	buffer.WriteString(" RETURNING id, created_at, updated_at")
	query_str := buffer.String()
	fmt.Println(query_str)
	rows, err := store.Pool.Query(query_str, value_map)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int
		var created_at, updated_at time.Time
		rows.Scan(&id, &created_at, &updated_at)
		fmt.Println(id, created_at, updated_at)
		model.FillMeta(id, created_at, updated_at)
	}
	return nil
}
func (store *ModelStore) DeleteModel(model Model) error {
	_, err := store.Pool.Exec("DELETE FROM "+store.TableName+" WHERE id = $1", model.GetId())
	return err
}

func ToSnake(in string) string {
	runes := []rune(in)
	length := len(runes)

	var out []rune
	for i := 0; i < length; i++ {
		if i > 0 && unicode.IsUpper(runes[i]) && ((i+1 < length && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(runes[i]))
	}
	return string(out)
}
