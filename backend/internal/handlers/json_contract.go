package handlers

import (
	"reflect"

	"github.com/gin-gonic/gin"
)

// writeJSON aplica el contrato transversal de colecciones antes de serializar una respuesta pública.
func writeJSON(c *gin.Context, status int, payload any) {
	normalized := initializeNilSlices(reflect.ValueOf(payload))
	if !normalized.IsValid() {
		c.JSON(status, nil)
		return
	}
	c.JSON(status, normalized.Interface())
}

// initializeNilSlices copia recursivamente un valor e inicializa únicamente sus slices nulos.
func initializeNilSlices(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		inner := initializeNilSlices(value.Elem())
		copy := reflect.New(value.Type()).Elem()
		copy.Set(inner)
		return copy
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		copy := reflect.New(value.Type().Elem())
		copy.Elem().Set(initializeNilSlices(value.Elem()))
		return copy
	case reflect.Slice:
		if value.IsNil() {
			return reflect.MakeSlice(value.Type(), 0, 0)
		}
		copy := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			copy.Index(index).Set(initializeNilSlices(value.Index(index)))
		}
		return copy
	case reflect.Array:
		copy := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			copy.Index(index).Set(initializeNilSlices(value.Index(index)))
		}
		return copy
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		copy := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			copy.SetMapIndex(iterator.Key(), initializeNilSlices(iterator.Value()))
		}
		return copy
	case reflect.Struct:
		copy := reflect.New(value.Type()).Elem()
		copy.Set(value)
		for index := 0; index < value.NumField(); index++ {
			if copy.Field(index).CanSet() && value.Type().Field(index).PkgPath == "" {
				copy.Field(index).Set(initializeNilSlices(value.Field(index)))
			}
		}
		return copy
	default:
		return value
	}
}
