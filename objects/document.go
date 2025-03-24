package objects

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getstingrai/dive/document"
	"github.com/risor-io/risor/object"
	"github.com/risor-io/risor/op"
)

var _ object.Object = &Document{}

type Document struct {
	doc document.Document
}

func (d *Document) Cost() int {
	return 0
}

func (d *Document) SetAttr(name string, value object.Object) error {
	return errors.New("not implemented")
}

func (d *Document) GetAttr(name string) (object.Object, bool) {
	switch name {
	case "content":
		return object.NewString(d.doc.Content()), true
	default:
		return nil, false
	}
}

func (d *Document) Type() object.Type {
	return "document"
}

func (d *Document) Value() document.Document {
	return d.doc
}

func (d *Document) Inspect() string {
	return fmt.Sprintf("document(name=%s)", d.doc.Name())
}

func (d *Document) Interface() interface{} {
	return d.doc
}

func (d *Document) String() string {
	return d.Inspect()
}

func (d *Document) Equals(other object.Object) object.Object {
	if other, ok := other.(*Document); ok {
		return object.NewBool(d.doc == other.doc)
	}
	return object.NewBool(false)
}

func (d *Document) IsTruthy() bool {
	return d.doc.Content() != ""
}

func (d *Document) RunOperation(opType op.BinaryOpType, right object.Object) object.Object {
	switch right := right.(type) {
	default:
		return object.TypeErrorf("type error: unsupported operation: %v on type %s", opType, right.Type())
	}
}

func (d *Document) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"name":         d.doc.Name(),
		"path":         d.doc.Path(),
		"content":      d.doc.Content(),
		"content_type": d.doc.ContentType(),
	})
}

func NewDocument(doc document.Document) *Document {
	return &Document{doc: doc}
}
