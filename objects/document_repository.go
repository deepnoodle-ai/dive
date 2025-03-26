package objects

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diveagents/dive"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/op"
)

var _ object.Object = &DocumentRepository{}

type DocumentRepository struct {
	repo dive.DocumentRepository
}

func (d *DocumentRepository) Cost() int {
	return 0
}

func (d *DocumentRepository) SetAttr(name string, value object.Object) error {
	return nil
}

func (d *DocumentRepository) GetAttr(name string) (object.Object, bool) {
	switch name {
	case "get":
		return object.NewBuiltin("get", func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) != 1 {
				return object.TypeErrorf("type error: get() takes exactly 1 argument")
			}
			name, errObj := object.AsString(args[0])
			if errObj != nil {
				return errObj
			}
			doc, err := d.repo.GetDocument(ctx, name)
			if err != nil {
				return object.NewError(err)
			}
			return NewDocument(doc)
		}), true
	default:
		return nil, false
	}
}

func (d *DocumentRepository) Type() object.Type {
	return "documents"
}

func (d *DocumentRepository) Value() dive.DocumentRepository {
	return d.repo
}

func (d *DocumentRepository) Inspect() string {
	return fmt.Sprintf("%v", d.repo)
}

func (d *DocumentRepository) Interface() interface{} {
	return d.repo
}

func (d *DocumentRepository) String() string {
	return d.Inspect()
}

func (d *DocumentRepository) Equals(other object.Object) object.Object {
	if other, ok := other.(*DocumentRepository); ok {
		return object.NewBool(d.repo == other.repo)
	}
	return object.NewBool(false)
}

func (d *DocumentRepository) IsTruthy() bool {
	return d.repo != nil
}

func (d *DocumentRepository) RunOperation(opType op.BinaryOpType, right object.Object) object.Object {
	switch right := right.(type) {
	default:
		return object.TypeErrorf("type error: unsupported operation: %v on type %s", opType, right.Type())
	}
}

func (d *DocumentRepository) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{})
}

func NewDocumentRepository(repo dive.DocumentRepository) *DocumentRepository {
	return &DocumentRepository{repo: repo}
}
