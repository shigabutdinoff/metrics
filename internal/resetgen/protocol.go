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

func traversalInterface(owner types.Type) *types.Interface {
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
