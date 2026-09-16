package resetgen

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"strings"
)

func (c *genericResetChecker) checkGeneratedArgument(t types.Type, legacy bool) error {
	if types.IsInterface(t) {
		return nil
	}
	selection := types.NewMethodSet(t).Lookup(nil, resetMethodName)
	if selection == nil || len(selection.Index()) == 1 {
		return nil
	}
	method := selection.Obj().(*types.Func)
	if sig := method.Signature(); sig.Params().Len() != 0 || sig.Results().Len() != 0 {
		return nil
	}
	owned, err := c.generatedMethod(method)
	if err != nil || !owned {
		return err
	}
	if types.Implements(t, c.protocol) || (!legacy && types.Implements(t, traversalInterface(t))) {
		adapter := types.NewMethodSet(t).Lookup(nil, resetWithVisitedMethodName)
		owned, err := c.generatedMethod(adapter.Obj().(*types.Func))
		if err != nil {
			return err
		}
		if !owned {
			return nil
		}
	}
	return fmt.Errorf("%s: унаследованный Reset типа %s теряет общую карту посещений; добавьте generate:reset или пользовательский адаптер ResetWithVisited",
		types.TypeString(t, nil), types.TypeString(method.Signature().Recv().Type(), nil))
}

func checkLegacyEmbedding(t types.Type) error {
	if types.IsInterface(t) || !types.Implements(t, traversalInterface(nil)) {
		return nil
	}
	selection := types.NewMethodSet(t).Lookup(nil, resetWithVisitedMethodName)
	method := selection.Obj().(*types.Func)
	_, pointerReceiver := types.Unalias(method.Signature().Recv().Type()).(*types.Pointer)
	fields := selection.Index()[:len(selection.Index())-1]
	current := t
	var path []string
	for i, index := range fields {
		if pointer, ok := types.Unalias(current).(*types.Pointer); ok {
			current = pointer.Elem()
		}
		field := current.Underlying().(*types.Struct).Field(index)
		path = append(path, field.Name())
		current = field.Type()
		if _, pointer := types.Unalias(current).(*types.Pointer); pointer && (i < len(fields)-1 || !pointerReceiver) {
			return fmt.Errorf("%s.ResetWithVisited: унаследованный legacy-протокол разыменовывает nil во встроенном поле %s; добавьте безопасный адаптер ResetWithVisited или перегенерируйте внешний модуль",
				types.TypeString(t, nil), strings.Join(path, "."))
		}
	}
	return nil
}

func (c *genericResetChecker) checkLegacyArgument(t types.Type) error {
	if types.IsInterface(t) || !types.Implements(t, traversalInterface(t)) {
		return nil
	}
	owned, err := c.generatedProtocol(t)
	if err != nil || !owned {
		return err
	}
	return fmt.Errorf("%s: новый ResetWithVisited теряет общую карту посещений в устаревшем контейнере; перегенерируйте внешний модуль контейнера", types.TypeString(t, nil))
}

func (c *genericResetChecker) generatedMethod(method *types.Func) (bool, error) {
	path, err := c.sources.path(method.Origin())
	if err != nil {
		return false, err
	}
	if filepath.Base(path) != genFile {
		return false, nil
	}
	if owned, ok := c.owned[path]; ok {
		return owned, nil
	}
	src, ok := c.overlay[path]
	if !ok {
		src, err = os.ReadFile(path)
		if err != nil {
			return false, err
		}
	}
	c.owned[path] = hasGenHeader(src)
	return c.owned[path], nil
}
