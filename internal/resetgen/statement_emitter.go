package resetgen

import (
	"fmt"
	"go/types"
	"strings"
)

type statementEmitter struct {
	marked       map[*types.TypeName]bool
	names        nameAllocator
	helpers      *resetHelpers
	declarations map[string]bool
	constraints  *constraintChecker
	visited      string
}

func (e *statementEmitter) emit(expr string, t types.Type, pointers map[*types.Pointer]bool) (string, error) {
	if err := e.constraints.checkType(t); err != nil {
		return "", err
	}
	if err := e.constraints.checkConditionalReset(t); err != nil {
		return "", err
	}
	if n, ok := types.Unalias(t).(*types.Named); ok && e.marked[n.Obj()] {
		return fmt.Sprintf("%s.%s(%s, &(%s))", operand(expr), resetWithVisitedMethodName, e.visited, expr), nil
	}
	if method := resetMethod(t); method != nil {
		value := "&(" + expr + ")"
		var valueType types.Type = types.NewPointer(t)
		if param, ok := types.Unalias(t).(*types.TypeParam); ok {
			if err := e.constraints.checkResetConstraint(param.Constraint()); err != nil {
				return "", err
			}
			value = expr
			valueType = t
			e.helpers.parameters[param] = true
		} else {
			if err := e.constraints.checkObject(method); err != nil {
				return "", err
			}
			_, pointerReceiver := types.Unalias(method.Signature().Recv().Type()).(*types.Pointer)
			if !pointerReceiver && types.Implements(t, traversalInterface(t)) {
				value = expr
				valueType = t
			}
		}
		if err := checkLegacyEmbedding(valueType); err != nil {
			return "", err
		}
		if e.helpers.value == "" {
			e.helpers.value = e.names.take("resetValue")
		}
		return fmt.Sprintf("%s(%s, %s)", e.helpers.value, value, e.visited), nil
	}
	if param, ok := types.Unalias(t).(*types.TypeParam); ok {
		if err := e.constraints.checkZeroConstraint(param.Constraint()); err != nil {
			return "", err
		}
		zero := e.names.take("resetZero")
		return fmt.Sprintf("{\nvar %s %s\n%s = %s\n}", zero, param.Obj().Name(), expr, zero), nil
	}

	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if pointers[u] {
			return "", fmt.Errorf("циклический тип указателя %s", types.TypeString(t, nil))
		}
		pointers[u] = true
		inner, err := e.emit("*"+expr, u.Elem(), pointers)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("if %s != nil {\n%s\n}", expr, inner), nil
	case *types.Slice:
		return fmt.Sprintf("%s = %s[:0]", expr, operand(expr)), nil
	case *types.Map:
		if e.declarations["clear"] {
			return "", fmt.Errorf("встроенный clear затенён объявлением пакета")
		}
		return fmt.Sprintf("clear(%s)", expr), nil
	case *types.Struct, *types.Array:
		if e.helpers.zero == "" {
			e.helpers.zero = e.names.take("resetZero")
		}
		return fmt.Sprintf("%s(&(%s))", e.helpers.zero, expr), nil
	}

	zero, err := zeroValue(t, e.declarations)
	if err != nil {
		return "", err
	}

	return expr + " = " + zero, nil
}

func operand(expr string) string {
	if strings.HasPrefix(expr, "*") {
		return "(" + expr + ")"
	}

	return expr
}

func resetMethod(t types.Type) *types.Func {
	t = types.Unalias(t)
	switch t := t.(type) {
	case *types.TypeParam:
	case *types.Named:
		if types.IsInterface(t) {
			return nil
		}
	default:
		return nil
	}

	obj, index, _ := types.LookupFieldOrMethod(t, true, nil, resetMethodName)
	fn, ok := obj.(*types.Func)
	if !ok || len(index) > 1 {
		return nil
	}

	sig := fn.Signature()
	if sig.Params().Len() == 0 && sig.Results().Len() == 0 {
		return fn
	}

	return nil
}

func zeroValue(t types.Type, declarations map[string]bool) (string, error) {
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			if declarations["false"] {
				return "", fmt.Errorf("предопределённый false затенён объявлением пакета")
			}
			return "false", nil
		case u.Info()&types.IsString != 0:
			return `""`, nil
		case u.Info()&types.IsNumeric != 0:
			return "0", nil
		}
	}

	return "nil", nil
}
