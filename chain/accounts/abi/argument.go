package abi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

type Argument struct {
	Name    string
	Type    Type
	Indexed bool
}

type Arguments []Argument

type ArgumentMarshaling struct {
	Name         string
	Type         string
	InternalType string
	Components   []ArgumentMarshaling
	Indexed      bool
}

func (argument *Argument) UnmarshalJSON(data []byte) error {
	var arg ArgumentMarshaling
	err := json.Unmarshal(data, &arg)
	if err != nil {
		return fmt.Errorf("argument json err: %v", err)
	}

	argument.Type, err = NewType(arg.Type, arg.InternalType, arg.Components)
	if err != nil {
		return err
	}
	argument.Name = arg.Name
	argument.Indexed = arg.Indexed

	return nil
}

func (arguments Arguments) LengthNonIndexed() int {
	out := 0
	for _, arg := range arguments {
		if !arg.Indexed {
			out++
		}
	}
	return out
}

func (arguments Arguments) NonIndexed() Arguments {
	var ret []Argument
	for _, arg := range arguments {
		if !arg.Indexed {
			ret = append(ret, arg)
		}
	}
	return ret
}

func (arguments Arguments) isTuple() bool {
	return len(arguments) > 1
}

func (arguments Arguments) Unpack(v interface{}, data []byte) error {
	if len(data) == 0 {
		if len(arguments) != 0 {
			return fmt.Errorf("abi: attempting to unmarshall an empty string while arguments are expected")
		} else {
			return nil
		}
	}

	if reflect.Ptr != reflect.ValueOf(v).Kind() {
		return fmt.Errorf("abi: Unpack(non-pointer %T)", v)
	}
	marshalledValues, err := arguments.UnpackValues(data)

	if err != nil {
		return err
	}
	if arguments.isTuple() {
		return arguments.unpackTuple(v, marshalledValues)
	}
	return arguments.unpackAtomic(v, marshalledValues[0])
}

func (arguments Arguments) UnpackIntoMap(v map[string]interface{}, data []byte) error {
	if len(data) == 0 {
		if len(arguments) != 0 {
			return fmt.Errorf("abi: attempting to unmarshall an empty string while arguments are expected")
		} else {
			return nil
		}
	}
	marshalledValues, err := arguments.UnpackValues(data)
	if err != nil {
		return err
	}
	return arguments.unpackIntoMap(v, marshalledValues)
}

func unpack(t *Type, dst interface{}, src interface{}) error {
	var (
		dstVal = reflect.ValueOf(dst).Elem()
		srcVal = reflect.ValueOf(src)
	)
	tuple, typ := false, t
	for {
		if typ.T == SliceTy || typ.T == ArrayTy {
			typ = typ.Elem
			continue
		}
		tuple = typ.T == TupleTy
		break
	}
	if !tuple {
		return set(dstVal, srcVal)
	}

	dstVal = indirectInterfaceOrPtr(dstVal)

	switch t.T {
	case TupleTy:
		if dstVal.Kind() != reflect.Struct {
			return fmt.Errorf("abi: invalid dst value for unpack, want struct, got %s", dstVal.Kind())
		}
		fieldmap, err := mapArgNamesToStructFields(t.TupleRawNames, dstVal)
		if err != nil {
			return err
		}
		for i, elem := range t.TupleElems {
			fname := fieldmap[t.TupleRawNames[i]]
			field := dstVal.FieldByName(fname)
			if !field.IsValid() {
				return fmt.Errorf("abi: field %s can't found in the given value", t.TupleRawNames[i])
			}
			if err := unpack(elem, field.Addr().Interface(), srcVal.Field(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	case SliceTy:
		if dstVal.Kind() != reflect.Slice {
			return fmt.Errorf("abi: invalid dst value for unpack, want slice, got %s", dstVal.Kind())
		}
		slice := reflect.MakeSlice(dstVal.Type(), srcVal.Len(), srcVal.Len())
		for i := 0; i < slice.Len(); i++ {
			if err := unpack(t.Elem, slice.Index(i).Addr().Interface(), srcVal.Index(i).Interface()); err != nil {
				return err
			}
		}
		dstVal.Set(slice)
	case ArrayTy:
		if dstVal.Kind() != reflect.Array {
			return fmt.Errorf("abi: invalid dst value for unpack, want array, got %s", dstVal.Kind())
		}
		array := reflect.New(dstVal.Type()).Elem()
		for i := 0; i < array.Len(); i++ {
			if err := unpack(t.Elem, array.Index(i).Addr().Interface(), srcVal.Index(i).Interface()); err != nil {
				return err
			}
		}
		dstVal.Set(array)
	}
	return nil
}

func (arguments Arguments) unpackIntoMap(v map[string]interface{}, marshalledValues []interface{}) error {

	if v == nil {
		return fmt.Errorf("abi: cannot unpack into a nil map")
	}

	for i, arg := range arguments.NonIndexed() {
		v[arg.Name] = marshalledValues[i]
	}
	return nil
}

func (arguments Arguments) unpackAtomic(v interface{}, marshalledValues interface{}) error {
	if arguments.LengthNonIndexed() == 0 {
		return nil
	}
	argument := arguments.NonIndexed()[0]
	elem := reflect.ValueOf(v).Elem()

	if elem.Kind() == reflect.Struct && argument.Type.T != TupleTy {
		fieldmap, err := mapArgNamesToStructFields([]string{argument.Name}, elem)
		if err != nil {
			return err
		}
		field := elem.FieldByName(fieldmap[argument.Name])
		if !field.IsValid() {
			return fmt.Errorf("abi: field %s can't be found in the given value", argument.Name)
		}
		return unpack(&argument.Type, field.Addr().Interface(), marshalledValues)
	}
	return unpack(&argument.Type, elem.Addr().Interface(), marshalledValues)
}

func (arguments Arguments) unpackTuple(v interface{}, marshalledValues []interface{}) error {
	var (
		value = reflect.ValueOf(v).Elem()
		typ   = value.Type()
		kind  = value.Kind()
	)
	if err := requireUnpackKind(value, typ, kind, arguments); err != nil {
		return err
	}

	var abi2struct map[string]string
	if kind == reflect.Struct {
		var (
			argNames []string
			err      error
		)
		for _, arg := range arguments.NonIndexed() {
			argNames = append(argNames, arg.Name)
		}
		abi2struct, err = mapArgNamesToStructFields(argNames, value)
		if err != nil {
			return err
		}
	}
	for i, arg := range arguments.NonIndexed() {
		switch kind {
		case reflect.Struct:
			field := value.FieldByName(abi2struct[arg.Name])
			if !field.IsValid() {
				return fmt.Errorf("abi: field %s can't be found in the given value", arg.Name)
			}
			if err := unpack(&arg.Type, field.Addr().Interface(), marshalledValues[i]); err != nil {
				return err
			}
		case reflect.Slice, reflect.Array:
			if value.Len() < i {
				return fmt.Errorf("abi: insufficient number of arguments for unpack, want %d, got %d", len(arguments), value.Len())
			}
			v := value.Index(i)
			if err := requireAssignable(v, reflect.ValueOf(marshalledValues[i])); err != nil {
				return err
			}
			if err := unpack(&arg.Type, v.Addr().Interface(), marshalledValues[i]); err != nil {
				return err
			}
		default:
			return fmt.Errorf("abi:[2] cannot unmarshal tuple in to %v", typ)
		}
	}
	return nil

}

func (arguments Arguments) UnpackValues(data []byte) ([]interface{}, error) {
	retval := make([]interface{}, 0, arguments.LengthNonIndexed())
	virtualArgs := 0
	for index, arg := range arguments.NonIndexed() {
		marshalledValue, err := toGoType((index+virtualArgs)*32, arg.Type, data)
		if arg.Type.T == ArrayTy && !isDynamicType(arg.Type) {

			virtualArgs += getTypeSize(arg.Type)/32 - 1
		} else if arg.Type.T == TupleTy && !isDynamicType(arg.Type) {

			virtualArgs += getTypeSize(arg.Type)/32 - 1
		}
		if err != nil {
			return nil, err
		}
		retval = append(retval, marshalledValue)
	}
	return retval, nil
}

func (arguments Arguments) PackValues(args []interface{}) ([]byte, error) {
	return arguments.Pack(args...)
}

func (arguments Arguments) Pack(args ...interface{}) ([]byte, error) {

	abiArgs := arguments
	if len(args) != len(abiArgs) {
		return nil, fmt.Errorf("argument count mismatch: %d for %d", len(args), len(abiArgs))
	}

	var variableInput []byte

	inputOffset := 0
	for _, abiArg := range abiArgs {
		inputOffset += getTypeSize(abiArg.Type)
	}
	var ret []byte
	for i, a := range args {
		input := abiArgs[i]

		packed, err := input.Type.pack(reflect.ValueOf(a))
		if err != nil {
			return nil, err
		}

		if isDynamicType(input.Type) {

			ret = append(ret, packNum(reflect.ValueOf(inputOffset))...)

			inputOffset += len(packed)

			variableInput = append(variableInput, packed...)
		} else {

			ret = append(ret, packed...)
		}
	}

	ret = append(ret, variableInput...)

	return ret, nil
}

func ToCamelCase(input string) string {
	parts := strings.Split(input, "_")
	for i, s := range parts {
		if len(s) > 0 {
			parts[i] = strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return strings.Join(parts, "")
}
