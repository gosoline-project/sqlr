package sqlr

import "reflect"

type mutationJournal struct {
	entries []fieldMutation
}

type fieldMutation struct {
	field    reflect.Value
	previous reflect.Value
}

func newMutationJournal() *mutationJournal {
	return &mutationJournal{
		entries: make([]fieldMutation, 0),
	}
}

func (j *mutationJournal) record(field reflect.Value) error {
	if j == nil {
		return nil
	}

	previous, err := cloneReflectValue(field)
	if err != nil {
		return err
	}

	j.entries = append(j.entries, fieldMutation{
		field:    field,
		previous: previous,
	})

	return nil
}

func (j *mutationJournal) restore() {
	if j == nil {
		return
	}

	for i := len(j.entries) - 1; i >= 0; i-- {
		entry := j.entries[i]
		entry.field.Set(entry.previous)
	}
}

func cloneReflectValue(value reflect.Value) (reflect.Value, error) {
	if !value.IsValid() {
		return reflect.Value{}, nil
	}

	clone := reflect.New(value.Type()).Elem()
	clone.Set(value)

	return clone, nil
}
