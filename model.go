package model

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
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
	DB        *sqlx.DB
	TableName string
}

func NewStore(db *sqlx.DB, tableName string) *ModelStore {
	store := &ModelStore{}
	sqlx.NameMapper = ToSnake
	store.DB = db
	store.TableName = tableName
	return store
}

func makeStringForExec(model Model) (string, string, map[string]interface{}) {
	var fieldBuffer, valueBuffer bytes.Buffer
	valueMap := make(map[string]interface{})
	v := reflect.Indirect(reflect.ValueOf(model))
	t := v.Type()
	numField := v.NumField()
	for posField := 0; posField < numField; posField++ {
		switch t.Field(posField).Name {
		case "Id", "UpdatedAt", "CreatedAt":
			continue
		}
		fieldBuffer.WriteString(ToSnake(t.Field(posField).Name) + ", ")
		valueBuffer.WriteString(":" + ToSnake(t.Field(posField).Name) + ", ")
		valueMap[ToSnake(t.Field(posField).Name)] = v.Field(posField).Interface()
	}
	field_str := fieldBuffer.String()
	value_str := valueBuffer.String()
	return field_str[:len(field_str)-2], value_str[:len(value_str)-2], valueMap
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
	rows, err := store.DB.NamedQuery(query_string, value_map)
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
	row := store.DB.QueryRowx("SELECT * FROM "+store.TableName+" WHERE id = $1", id)
	err := row.StructScan(model)
	switch err {
	case sql.ErrNoRows:
		fmt.Println("No rows returned")
	}
	return err
}

func (store *ModelStore) LoadModels(idList []int, modelSlice []Model) []Model {

	id_list_str := ""
	for _, id := range idList {
		id_list_str += strconv.Itoa(id) + ","
	}
	rows, err := store.DB.Queryx("SELECT * FROM " + store.TableName + " WHERE id IN (" + id_list_str[:len(id_list_str)-1] + ") ORDER BY id")
	switch err {
	case nil:
	case sql.ErrNoRows:
		fmt.Println("No rows returned")
		return nil
	default:
		panic(err)
	}
	index := 0
	for rows.Next() {
		rows.StructScan(modelSlice[index])
		index++
	}
	return modelSlice
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
	rows, err := store.DB.NamedQuery(query_str, value_map)
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
	_, err := store.DB.Exec("DELETE FROM "+store.TableName+" WHERE id = $1", model.GetId())
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
