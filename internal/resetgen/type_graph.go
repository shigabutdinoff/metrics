package resetgen

import "go/types"

type typeGraphVisitor func(types.Type) error

func walkTypeGraph(t types.Type, visited map[types.Type]bool, visit typeGraphVisitor) error {
	if t == nil || visited[t] {
		return nil
	}
	visited[t] = true
	if err := visit(t); err != nil {
		return err
	}
	switch t := t.(type) {
	case *types.Alias:
		return walkTypeGraph(types.Unalias(t), visited, visit)
	case *types.Named:
		return walkTypeGraph(t.Underlying(), visited, visit)
	case *types.Pointer:
		return walkTypeGraph(t.Elem(), visited, visit)
	case *types.Array:
		return walkTypeGraph(t.Elem(), visited, visit)
	case *types.Slice:
		return walkTypeGraph(t.Elem(), visited, visit)
	case *types.Map:
		if err := walkTypeGraph(t.Key(), visited, visit); err != nil {
			return err
		}
		return walkTypeGraph(t.Elem(), visited, visit)
	case *types.Chan:
		return walkTypeGraph(t.Elem(), visited, visit)
	case *types.Struct:
		for field := range t.Fields() {
			if err := walkTypeGraph(field.Type(), visited, visit); err != nil {
				return err
			}
		}
	case *types.Signature:
		if err := walkTypeGraph(t.Params(), visited, visit); err != nil {
			return err
		}
		return walkTypeGraph(t.Results(), visited, visit)
	case *types.Tuple:
		for variable := range t.Variables() {
			if err := walkTypeGraph(variable.Type(), visited, visit); err != nil {
				return err
			}
		}
	}
	return nil
}
