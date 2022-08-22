package clice

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"reflect"
	"strconv"
)

type value reflect.Value

func reflectValue(v reflect.Value) value {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	return value(v)
}

func (v value) Value() reflect.Value {
	return reflect.Value(v)
}

func (ss *SpreadSheet) evaluate(ctx context.Context, cell *Cell, expression ast.Expr) (value, error) {
	fmt.Printf("%#[1]v\n", expression)

	switch ex := expression.(type) {
	case *ast.BasicLit:
		return evaluateBasicLiteral(ex)
	case *ast.CallExpr:
		return ss.evaluateCallExpression(ctx, cell, ex)
	case *ast.UnaryExpr:
		return ss.evaluateUnaryExpression(ctx, cell, ex)
	case *ast.BinaryExpr:
		return ss.evaluateBinaryExpression(ctx, cell, ex)
	}

	return value{}, fmt.Errorf("unhandled expression type %[1]T: %[1]s", expression)
}

func evaluateBasicLiteral(lit *ast.BasicLit) (value, error) {
	switch lit.Kind {
	case token.INT:
		v, err := strconv.ParseInt(lit.Value, 10, 64)
		if err != nil {
			return value{}, err
		}
		return reflectValue(reflect.ValueOf(v)), nil
	case token.FLOAT:
		v, err := strconv.ParseFloat(lit.Value, 64)
		if err != nil {
			return value{}, err
		}
		return reflectValue(reflect.ValueOf(v)), nil
	case token.IMAG:
		v, err := strconv.ParseComplex(lit.Value, 128)
		if err != nil {
			return value{}, err
		}
		return reflectValue(reflect.ValueOf(v)), nil
	case token.CHAR:
		s := lit.Value[1 : len(lit.Value)-1]
		var v rune
		for _, c := range s {
			v = c
			break
		}
		return reflectValue(reflect.ValueOf(v)), nil
	case token.STRING:
		v, err := strconv.Unquote(lit.Value)
		if err != nil {
			return value{}, err
		}
		return reflectValue(reflect.ValueOf(v)), nil
	default:
		return value{}, fmt.Errorf("failed to parse basi literal %s", lit.Kind)
	}
}

func (ss *SpreadSheet) evaluateCallExpression(ctx context.Context, cell *Cell, exp *ast.CallExpr) (value, error) {
	switch fn := exp.Fun.(type) {
	default:
		return value{}, fmt.Errorf("could not call %s", printNode(exp.Fun))

	case *ast.Ident:
		switch fn.Name {
		case "cell":
			intType := reflect.TypeOf(0)
			colVal, err := ss.evaluate(ctx, cell, exp.Args[0])
			if err != nil {
				return value{}, err
			}
			if !colVal.Value().CanConvert(intType) {
				return value{}, fmt.Errorf("column must be of type int")
			}
			column := colVal.Value().Convert(reflect.TypeOf(0)).Interface().(int)
			rowVal, err := ss.evaluate(ctx, cell, exp.Args[1])
			if err != nil {
				return value{}, err
			}
			if !rowVal.Value().CanConvert(intType) {
				return value{}, fmt.Errorf("row must be of type int")
			}
			row := rowVal.Value().Convert(reflect.TypeOf(0)).Interface().(int)

			refCell, found := ss.FindCell(ctx, column, row)
			if !found {
				return value{}, fmt.Errorf("not found")
			}
			if refCell.references(ctx, cell) {
				return value{}, fmt.Errorf("recursive reference")
			}
			refCell.attach(ctx, cell)

			return refCell.value, nil
		}

		o, err := ss.lookUp(ctx, fn.Name)
		if err != nil {
			return value{}, err
		}

		switch object := o.(type) {
		default:
			return value{}, errUnknownScopeValueType(fn.Name, object)

		case reflect.Type:
			namedType := object

			if len(exp.Args) > 1 {
				return value{}, fmt.Errorf("too many arguments to conversion to %s", fn.Name)
			}
			if len(exp.Args) == 0 {
				return value{}, fmt.Errorf("missing argument to conversion to %s", fn.Name)
			}

			val, err := ss.evaluate(ctx, cell, exp.Args[0])
			if err != nil {
				return value{}, err
			}
			if !val.Value().Type().ConvertibleTo(namedType) {
				return value{}, fmt.Errorf("cannot convert %s to type %s", val.Value().Type().Name(), fn.Name)
			}

			return reflectValue(val.Value().Convert(namedType)), nil

		case reflect.Value:
			function := object

			if function.Kind() != reflect.Func {
				return value{}, fmt.Errorf("could not call %s it is a %s", fn.Name, function.Type().Name())
			}

			var args []reflect.Value

			inCountAdjustment := 0
			if function.Type().NumIn() > 0 {
				in0Type := function.Type().In(0)
				if in0Type.PkgPath() == "context" &&
					in0Type.Name() == "Context" {
					inCountAdjustment++
					args = append(args, reflect.ValueOf(ctx))
				}
			}

			for i, arg := range exp.Args {
				// TODO: maybe support wrapping functions with multiple returns like html/template.Must does
				val, err := ss.evaluate(ctx, cell, arg)
				if err != nil {
					return value{}, err
				}

				v := val.Value()
				if v.Kind() == reflect.Slice && v.Len() == 0 {
					v = reflect.MakeSlice(function.Type().In(i+inCountAdjustment), 0, 0)
				}

				if v.Type().ConvertibleTo(function.Type().In(i + inCountAdjustment)) {
					v = v.Convert(function.Type().In(i + inCountAdjustment))
				} else {
					return value{}, fmt.Errorf("cannot use %v (type %s) as type %s in argument to %s", v.Interface(), v.Type(), function.Type().In(i+inCountAdjustment), fn.Name)
				}

				args = append(args, v)
			}

			if function.Type().NumIn() < len(args) {
				return value{}, fmt.Errorf("too many arguments in call to %s", fn.Name)
			}

			if function.Type().NumIn() > len(args) {
				return value{}, fmt.Errorf("not enough arguments in call to %s", fn.Name)
			}

			results, err := func() (_ []reflect.Value, err error) {
				defer func() {
					r := recover()
					if r != nil {
						err = fmt.Errorf("panic during call to %s: %s", fn.Name, r)
					}
				}()
				results := function.Call(args)
				return results, nil
			}()
			if err != nil {
				return value{}, err
			}
			switch len(results) {
			case 1:
				return reflectValue(results[0]), nil
			case 2:
				errVal := results[1].Interface()
				if errVal != nil {
					return value{}, err.(error)
				}
				return reflectValue(results[0]), nil
			default:
				return value{}, fmt.Errorf("the function must have either one or two results if it has two the second must implement error")
			}
		}
	}
}

func (ss *SpreadSheet) evaluateUnaryExpression(ctx context.Context, cell *Cell, exp *ast.UnaryExpr) (value, error) {
	v, err := ss.evaluate(ctx, cell, exp.X)
	if err != nil {
		return value{}, err
	}
	val := v.Value()

	var result interface{}

	switch exp.Op {
	case token.ADD: // +
		switch val.Kind() {
		default:
			return value{}, errInvalidXOperation(exp.Op, val)
		case reflect.Int:
			result = +int(val.Int())
		case reflect.Int8:
			result = +int8(val.Int())
		case reflect.Int16:
			result = +int16(val.Int())
		case reflect.Int32:
			result = +int32(val.Int())
		case reflect.Int64:
			result = +val.Int()

		case reflect.Uint:
			result = +uint(val.Uint())
		case reflect.Uint8:
			result = +uint8(val.Uint())
		case reflect.Uint16:
			result = +uint16(val.Uint())
		case reflect.Uint32:
			result = +uint32(val.Uint())
		case reflect.Uint64:
			result = +val.Uint()

		case reflect.Float32:
			result = +float32(val.Float())
		case reflect.Float64:
			result = +val.Float()

		case reflect.Complex64:
			result = +complex64(val.Complex())
		case reflect.Complex128:
			result = +val.Complex()
		}
	case token.SUB: // -
		switch val.Kind() {
		default:
			return value{}, errInvalidXOperation(exp.Op, val)
		case reflect.Int:
			result = -int(val.Int())
		case reflect.Int8:
			result = -int8(val.Int())
		case reflect.Int16:
			result = -int16(val.Int())
		case reflect.Int32:
			result = -int32(val.Int())
		case reflect.Int64:
			result = -val.Int()

		case reflect.Uint:
			result = -uint(val.Uint())
		case reflect.Uint8:
			result = -uint8(val.Uint())
		case reflect.Uint16:
			result = -uint16(val.Uint())
		case reflect.Uint32:
			result = -uint32(val.Uint())
		case reflect.Uint64:
			result = -val.Uint()

		case reflect.Float32:
			result = -float32(val.Float())
		case reflect.Float64:
			result = -val.Float()

		case reflect.Complex64:
			result = -complex64(val.Complex())
		case reflect.Complex128:
			result = -val.Complex()
		}
	case token.XOR: // ^
		switch val.Kind() {
		default:
			return value{}, errInvalidXOperation(exp.Op, val)
		case reflect.Int:
			result = ^int(val.Int())
		case reflect.Int8:
			result = ^int8(val.Int())
		case reflect.Int16:
			result = ^int16(val.Int())
		case reflect.Int32:
			result = ^int32(val.Int())
		case reflect.Int64:
			result = ^val.Int()

		case reflect.Uint:
			result = ^uint(val.Uint())
		case reflect.Uint8:
			result = ^uint8(val.Uint())
		case reflect.Uint16:
			result = ^uint16(val.Uint())
		case reflect.Uint32:
			result = ^uint32(val.Uint())
		case reflect.Uint64:
			result = ^val.Uint()
		}
	case token.NOT: // !
		switch val.Kind() {
		default:
			return value{}, errInvalidXOperation(exp.Op, val)
		case reflect.Bool:
			result = !val.Interface().(bool)
		}
	}

	return reflectValue(reflect.ValueOf(result)), nil
}

func (ss *SpreadSheet) evaluateBinaryExpression(ctx context.Context, cell *Cell, exp *ast.BinaryExpr) (value, error) {
	xValue, err := ss.evaluate(ctx, cell, exp.X)
	if err != nil {
		return value{}, err
	}
	yValue, err := ss.evaluate(ctx, cell, exp.Y)
	if err != nil {
		return value{}, err
	}
	xVal, yVal := xValue.Value(), yValue.Value()

	if exp.Op != token.SHL && exp.Op != token.SHR && !xVal.Type().AssignableTo(yVal.Type()) {
		return value{}, fmt.Errorf("could not convert type: %s", xVal.Type().Name())
	}

	invalidXOperationErr := func() error {
		return fmt.Errorf("invalid operation (operator %s not defined on %v of kind %s)", exp.Op, xVal.Interface(), xVal.Type())
	}

	var result interface{}

	switch exp.Op {
	case token.ADD: // +
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) + int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) + int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) + int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) + int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() + yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) + uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) + uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) + uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) + uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() + yVal.Uint()

		case reflect.Float32:
			result = float32(xVal.Float()) + float32(yVal.Float())
		case reflect.Float64:
			result = xVal.Float() + yVal.Float()

		case reflect.Complex64:
			result = complex64(xVal.Complex()) + complex64(yVal.Complex())
		case reflect.Complex128:
			result = xVal.Complex() + yVal.Complex()

		case reflect.String:
			result = xVal.String() + yVal.String()
		}
	case token.SUB: // -
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) - int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) - int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) - int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) - int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() - yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) - uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) - uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) - uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) - uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() - yVal.Uint()

		case reflect.Float32:
			result = float32(xVal.Float()) - float32(yVal.Float())
		case reflect.Float64:
			result = xVal.Float() - yVal.Float()

		case reflect.Complex64:
			result = complex64(xVal.Complex()) - complex64(yVal.Complex())
		case reflect.Complex128:
			result = xVal.Complex() - yVal.Complex()
		}
	case token.MUL: // *
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) * int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) * int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) * int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) * int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() * yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) * uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) * uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) * uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) * uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() * yVal.Uint()

		case reflect.Float32:
			result = float32(xVal.Float()) * float32(yVal.Float())
		case reflect.Float64:
			result = xVal.Float() * yVal.Float()

		case reflect.Complex64:
			result = complex64(xVal.Complex()) * complex64(yVal.Complex())
		case reflect.Complex128:
			result = xVal.Complex() * yVal.Complex()
		}
	case token.QUO: // /
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) / int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) / int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) / int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) / int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() / yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) / uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) / uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) / uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) / uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() / yVal.Uint()

		case reflect.Float32:
			result = float32(xVal.Float()) / float32(yVal.Float())
		case reflect.Float64:
			result = xVal.Float() / yVal.Float()

		case reflect.Complex64:
			result = complex64(xVal.Complex()) / complex64(yVal.Complex())
		case reflect.Complex128:
			result = xVal.Complex() / yVal.Complex()
		}
	case token.REM: // %
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) % int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) % int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) % int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) % int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() % yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) % uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) % uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) % uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) % uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() % yVal.Uint()
		}

	case token.AND: // &
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) & int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) & int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) & int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) & int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() & yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) & uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) & uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) & uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) & uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() & yVal.Uint()
		}
	case token.OR: // |
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) | int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) | int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) | int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) | int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() | yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) | uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) | uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) | uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) | uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() | yVal.Uint()
		}
	case token.XOR: // ^
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) ^ int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) ^ int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) ^ int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) ^ int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() ^ yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) ^ uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) ^ uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) ^ uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) ^ uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() ^ yVal.Uint()
		}
	case token.AND_NOT: // &^
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = int(xVal.Int()) &^ int(yVal.Int())
		case reflect.Int8:
			result = int8(xVal.Int()) &^ int8(yVal.Int())
		case reflect.Int16:
			result = int16(xVal.Int()) &^ int16(yVal.Int())
		case reflect.Int32:
			result = int32(xVal.Int()) &^ int32(yVal.Int())
		case reflect.Int64:
			result = xVal.Int() &^ yVal.Int()

		case reflect.Uint:
			result = uint(xVal.Uint()) &^ uint(yVal.Uint())
		case reflect.Uint8:
			result = uint8(xVal.Uint()) &^ uint8(yVal.Uint())
		case reflect.Uint16:
			result = uint16(xVal.Uint()) &^ uint16(yVal.Uint())
		case reflect.Uint32:
			result = uint32(xVal.Uint()) &^ uint32(yVal.Uint())
		case reflect.Uint64:
			result = xVal.Uint() &^ yVal.Uint()
		}

	case token.SHL, token.SHR: // <<, >>
		var shift uint64

		switch yVal.Kind() {
		case reflect.Uint:
			shift = yVal.Uint()
		case reflect.Uint8:
			shift = yVal.Uint()
		case reflect.Uint16:
			shift = yVal.Uint()
		case reflect.Uint32:
			shift = yVal.Uint()
		case reflect.Uint64:
			shift = yVal.Uint()
		}

		if exp.Op == token.SHL {
			switch xVal.Kind() {
			default:
				return value{}, invalidXOperationErr()
			case reflect.Int:
				result = int(xVal.Int()) << int(shift)
			case reflect.Int8:
				result = int8(xVal.Int()) << int8(shift)
			case reflect.Int16:
				result = int16(xVal.Int()) << int16(shift)
			case reflect.Int32:
				result = int32(xVal.Int()) << int32(shift)
			case reflect.Int64:
				result = xVal.Int() << shift

			case reflect.Uint:
				result = uint(xVal.Uint()) << uint(shift)
			case reflect.Uint8:
				result = uint8(xVal.Uint()) << uint8(shift)
			case reflect.Uint16:
				result = uint16(xVal.Uint()) << uint16(shift)
			case reflect.Uint32:
				result = uint32(xVal.Uint()) << uint32(shift)
			case reflect.Uint64:
				result = xVal.Uint() << shift
			}
		} else {
			switch xVal.Kind() {
			default:
				return value{}, invalidXOperationErr()
			case reflect.Int:
				result = int(xVal.Int()) >> int(shift)
			case reflect.Int8:
				result = int8(xVal.Int()) >> int8(shift)
			case reflect.Int16:
				result = int16(xVal.Int()) >> int16(shift)
			case reflect.Int32:
				result = int32(xVal.Int()) >> int32(shift)
			case reflect.Int64:
				result = xVal.Int() >> shift

			case reflect.Uint:
				result = uint(xVal.Uint()) >> uint(shift)
			case reflect.Uint8:
				result = uint8(xVal.Uint()) >> uint8(shift)
			case reflect.Uint16:
				result = uint16(xVal.Uint()) >> uint16(shift)
			case reflect.Uint32:
				result = uint32(xVal.Uint()) >> uint32(shift)
			case reflect.Uint64:
				result = xVal.Uint() >> shift
			}
		}

	case token.LAND: // &&
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Bool:
			result = xVal.Bool() && yVal.Bool()
		}
	case token.LOR: // ||
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Bool:
			result = xVal.Bool() || yVal.Bool()
		}

	case token.EQL: // ==
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = xVal.Int() == yVal.Int()
		case reflect.Int8:
			result = xVal.Int() == yVal.Int()
		case reflect.Int16:
			result = xVal.Int() == yVal.Int()
		case reflect.Int32:
			result = xVal.Int() == yVal.Int()
		case reflect.Int64:
			result = xVal.Int() == yVal.Int()

		case reflect.Uint:
			result = xVal.Uint() == yVal.Uint()
		case reflect.Uint8:
			result = xVal.Uint() == yVal.Uint()
		case reflect.Uint16:
			result = xVal.Uint() == yVal.Uint()
		case reflect.Uint32:
			result = xVal.Uint() == yVal.Uint()
		case reflect.Uint64:
			result = xVal.Uint() == yVal.Uint()

		case reflect.Float32:
			result = xVal.Float() == yVal.Float()
		case reflect.Float64:
			result = xVal.Float() == yVal.Float()

		case reflect.Complex64:
			result = xVal.Complex() == yVal.Complex()
		case reflect.Complex128:
			result = xVal.Complex() == yVal.Complex()

		case reflect.String:
			result = xVal.String() == yVal.String()

		case reflect.Bool:
			result = xVal.Bool() == yVal.Bool()
		}
	case token.NEQ: // !=
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = xVal.Int() != yVal.Int()
		case reflect.Int8:
			result = xVal.Int() != yVal.Int()
		case reflect.Int16:
			result = xVal.Int() != yVal.Int()
		case reflect.Int32:
			result = xVal.Int() != yVal.Int()
		case reflect.Int64:
			result = xVal.Int() != yVal.Int()

		case reflect.Uint:
			result = xVal.Uint() != yVal.Uint()
		case reflect.Uint8:
			result = xVal.Uint() != yVal.Uint()
		case reflect.Uint16:
			result = xVal.Uint() != yVal.Uint()
		case reflect.Uint32:
			result = xVal.Uint() != yVal.Uint()
		case reflect.Uint64:
			result = xVal.Uint() != yVal.Uint()

		case reflect.Float32:
			result = xVal.Float() != yVal.Float()
		case reflect.Float64:
			result = xVal.Float() != yVal.Float()

		case reflect.Complex64:
			result = xVal.Complex() != yVal.Complex()
		case reflect.Complex128:
			result = xVal.Complex() != yVal.Complex()

		case reflect.String:
			result = xVal.String() != yVal.String()

		case reflect.Bool:
			result = xVal.Bool() != yVal.Bool()
		}

		// TODO: implement comparable for structures, interfaces, arrays, and slices
		// - Interface values are comparable. Two interface values are equal if they have identical dynamic types and equal dynamic values or if both have value nil.
		// - Array values are comparable if values of the array element type are comparable. Two array values are equal if their corresponding elements are equal.
		// - Struct values are comparable if all their fields are comparable. Two struct values are equal if their corresponding non-blank fields are equal.
		// - Channel values are comparable. Two channel values are equal if they were created by the same call to make or if both have value nil.
		// - Pointer values are comparable. Two pointer values are equal if they point to the same variable or if both have value nil. Pointers to distinct zero-size variables may or may not be equal.

	case token.LSS: // <
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = xVal.Int() < yVal.Int()
		case reflect.Int8:
			result = xVal.Int() < yVal.Int()
		case reflect.Int16:
			result = xVal.Int() < yVal.Int()
		case reflect.Int32:
			result = xVal.Int() < yVal.Int()
		case reflect.Int64:
			result = xVal.Int() < yVal.Int()

		case reflect.Uint:
			result = xVal.Uint() < yVal.Uint()
		case reflect.Uint8:
			result = xVal.Uint() < yVal.Uint()
		case reflect.Uint16:
			result = xVal.Uint() < yVal.Uint()
		case reflect.Uint32:
			result = xVal.Uint() < yVal.Uint()
		case reflect.Uint64:
			result = xVal.Uint() < yVal.Uint()

		case reflect.Float32:
			result = xVal.Float() < yVal.Float()
		case reflect.Float64:
			result = xVal.Float() < yVal.Float()

		case reflect.String:
			result = xVal.String() < yVal.String()
		}
	case token.GTR: // >
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = xVal.Int() > yVal.Int()
		case reflect.Int8:
			result = xVal.Int() > yVal.Int()
		case reflect.Int16:
			result = xVal.Int() > yVal.Int()
		case reflect.Int32:
			result = xVal.Int() > yVal.Int()
		case reflect.Int64:
			result = xVal.Int() > yVal.Int()

		case reflect.Uint:
			result = xVal.Uint() > yVal.Uint()
		case reflect.Uint8:
			result = xVal.Uint() > yVal.Uint()
		case reflect.Uint16:
			result = xVal.Uint() > yVal.Uint()
		case reflect.Uint32:
			result = xVal.Uint() > yVal.Uint()
		case reflect.Uint64:
			result = xVal.Uint() > yVal.Uint()

		case reflect.Float32:
			result = xVal.Float() > yVal.Float()
		case reflect.Float64:
			result = xVal.Float() > yVal.Float()

		case reflect.String:
			result = xVal.String() > yVal.String()
		}
	case token.LEQ: // <=
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = xVal.Int() <= yVal.Int()
		case reflect.Int8:
			result = xVal.Int() <= yVal.Int()
		case reflect.Int16:
			result = xVal.Int() <= yVal.Int()
		case reflect.Int32:
			result = xVal.Int() <= yVal.Int()
		case reflect.Int64:
			result = xVal.Int() <= yVal.Int()

		case reflect.Uint:
			result = xVal.Uint() <= yVal.Uint()
		case reflect.Uint8:
			result = xVal.Uint() <= yVal.Uint()
		case reflect.Uint16:
			result = xVal.Uint() <= yVal.Uint()
		case reflect.Uint32:
			result = xVal.Uint() <= yVal.Uint()
		case reflect.Uint64:
			result = xVal.Uint() <= yVal.Uint()

		case reflect.Float32:
			result = xVal.Float() <= yVal.Float()
		case reflect.Float64:
			result = xVal.Float() <= yVal.Float()

		case reflect.String:
			result = xVal.String() <= yVal.String()
		}
	case token.GEQ: // >=
		switch xVal.Kind() {
		default:
			return value{}, invalidXOperationErr()
		case reflect.Int:
			result = xVal.Int() >= yVal.Int()
		case reflect.Int8:
			result = xVal.Int() >= yVal.Int()
		case reflect.Int16:
			result = xVal.Int() >= yVal.Int()
		case reflect.Int32:
			result = xVal.Int() >= yVal.Int()
		case reflect.Int64:
			result = xVal.Int() >= yVal.Int()

		case reflect.Uint:
			result = xVal.Uint() >= yVal.Uint()
		case reflect.Uint8:
			result = xVal.Uint() >= yVal.Uint()
		case reflect.Uint16:
			result = xVal.Uint() >= yVal.Uint()
		case reflect.Uint32:
			result = xVal.Uint() >= yVal.Uint()
		case reflect.Uint64:
			result = xVal.Uint() >= yVal.Uint()

		case reflect.Float32:
			result = xVal.Float() >= yVal.Float()
		case reflect.Float64:
			result = xVal.Float() >= yVal.Float()

		case reflect.String:
			result = xVal.String() >= yVal.String()
		}
	}

	return reflectValue(reflect.ValueOf(result)), nil
}

func errInvalidXOperation(op token.Token, val reflect.Value) error {
	return fmt.Errorf("invalid operation (operator %s not defined on %v of kind %s)", op, val.Interface(), val.Type())
}

func errUnknownIdentifier(s string) error {
	return fmt.Errorf("undefined: %s", s)
}

func errUnknownScopeValueType(identification string, v interface{}) error {
	return fmt.Errorf("unsupported identifier type for %s: %T", identification, v)
}

func printNode(node ast.Node) string {
	if node == nil {
		return ""
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), node); err != nil {
		return fmt.Sprintf("/* could not print AST node: %s */", err)
	}

	return buf.String()
}

func builtinTypes(scope map[string]kinder) {
	for _, t := range []reflect.Type{
		reflect.TypeOf(0), // int
		reflect.TypeOf(int8(0)),
		reflect.TypeOf(int16(0)),
		reflect.TypeOf(int32(0)),
		reflect.TypeOf(int64(0)),

		reflect.TypeOf(uint(0)),
		reflect.TypeOf(uint8(0)),
		reflect.TypeOf(uint16(0)),
		reflect.TypeOf(uint32(0)),
		reflect.TypeOf(uint64(0)),

		reflect.TypeOf(float32(0)),
		reflect.TypeOf(float64(0)),

		reflect.TypeOf(""), // string

		reflect.TypeOf(complex64(0)),
		reflect.TypeOf(complex128(0)),

		reflect.TypeOf(false), // bool
	} {
		scope[t.Name()] = t
	}
}
