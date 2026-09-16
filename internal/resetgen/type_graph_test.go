package resetgen

import (
	"go/token"
	"go/types"
	"testing"
)

func TestWalkTypeGraphVisitsRecursiveContainersOnce(t *testing.T) {
	node := types.NewNamed(
		types.NewTypeName(token.NoPos, nil, "Node", nil),
		types.NewStruct(nil, nil),
		nil,
	)
	alias := types.NewAlias(
		types.NewTypeName(token.NoPos, nil, "NodeAlias", nil),
		types.NewPointer(node),
	)
	payload := types.NewMap(
		types.Typ[types.String],
		types.NewSlice(types.NewArray(types.Typ[types.Int], 2)),
	)
	node.SetUnderlying(types.NewStruct(
		[]*types.Var{
			types.NewField(token.NoPos, nil, "Next", types.NewPointer(node), false),
			types.NewField(token.NoPos, nil, "Payload", payload, false),
			types.NewField(token.NoPos, nil, "Alias", alias, false),
		},
		nil,
	))

	visits := make(map[types.Type]int)
	err := walkTypeGraph(node, make(map[types.Type]bool), func(current types.Type) error {
		visits[current]++
		return nil
	})
	if err != nil {
		t.Fatalf("walk type graph: %v", err)
	}
	for current, count := range visits {
		if count != 1 {
			t.Errorf("type %s visited %d times", types.TypeString(current, nil), count)
		}
	}
	for _, current := range []types.Type{node, node.Underlying(), alias, types.Unalias(alias), payload, payload.Key(), payload.Elem()} {
		if visits[current] != 1 {
			t.Errorf("type %s was not visited", types.TypeString(current, nil))
		}
	}
}
