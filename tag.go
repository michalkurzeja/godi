package di

type TagID string

type Tag struct {
	id     TagID
	params map[string]any
}

func NewTag(id TagID) *Tag {
	return &Tag{id: id, params: map[string]any{}}
}

func (t *Tag) ID() TagID {
	return t.id
}

func (t *Tag) AddParam(name string, val any) *Tag {
	t.params[name] = val
	return t
}

func (t *Tag) GetParam(name string) any {
	return t.params[name]
}

func (t *Tag) HasParam(name string) bool {
	_, ok := t.params[name]
	return ok
}

func (t *Tag) Params() map[string]any {
	return t.params
}

func (t *Tag) String() string {
	return string(t.id)
}
