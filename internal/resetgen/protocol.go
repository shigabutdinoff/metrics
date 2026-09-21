package resetgen

import (
	"go/token"
	"go/types"
)

const (
	resetMethodName            = "Reset"
	resetWithVisitedMethodName = "ResetWithVisited"
)

var resetProtocolMethodNames = [...]string{resetMethodName, resetWithVisitedMethodName}

// needsTraversal сообщает, доберётся ли сброс типа до вложенных сбрасываемых.
func needsTraversal(n *types.Named, marked, traversal map[*types.TypeName]bool) bool {
	if n.TypeParams().Len() > 0 {
		return true
	}
	s, ok := n.Underlying().(*types.Struct)
	if !ok {
		return true
	}
	for f := range s.Fields() {
		if f.Name() == "_" {
			continue
		}
		if fieldNeedsTraversal(f.Type(), marked, traversal, make(map[*types.Pointer]bool)) {
			return true
		}
	}
	return false
}

// fieldNeedsTraversal повторяет ветвление statementEmitter.emit для поля.
func fieldNeedsTraversal(t types.Type, marked, traversal map[*types.TypeName]bool, pointers map[*types.Pointer]bool) bool {
	if n, ok := types.Unalias(t).(*types.Named); ok && marked[n.Obj()] {
		return traversal[n.Obj()]
	}
	if resetMethod(t) != nil {
		return true
	}
	pointer, ok := t.Underlying().(*types.Pointer)
	if !ok || pointers[pointer] {
		return false
	}
	pointers[pointer] = true

	return fieldNeedsTraversal(pointer.Elem(), marked, traversal, pointers)
}

func newTraversalInterface(owner types.Type) *types.Interface {
	reset := types.NewFunc(token.NoPos, nil, resetMethodName, types.NewSignatureType(nil, nil, nil, nil, nil, false))
	resetter := types.NewInterfaceType([]*types.Func{reset}, nil).Complete()
	visited := types.NewMap(types.NewInterfaceType(nil, nil).Complete(), types.NewStruct(nil, nil))
	parameters := []*types.Var{types.NewVar(token.NoPos, nil, "visited", visited), types.NewVar(token.NoPos, nil, "original", resetter)}
	if owner != nil {
		parameters = append(parameters, types.NewVar(token.NoPos, nil, "owner", types.NewSlice(owner)))
	}
	method := types.NewFunc(token.NoPos, nil, resetWithVisitedMethodName, types.NewSignatureType(nil, nil, nil, types.NewTuple(parameters...), nil, owner != nil))
	return types.NewInterfaceType([]*types.Func{method}, nil).Complete()
}
