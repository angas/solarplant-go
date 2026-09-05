package maybe

import "database/sql"

type Maybe[T any] struct {
	value T
	valid bool
}

func Some[T any](value T) Maybe[T] {
	return Maybe[T]{
		value: value,
		valid: true,
	}
}

func None[T any]() Maybe[T] {
	return Maybe[T]{
		valid: false,
	}
}

// FromSQL preserves the value and validity of a nullable SQL value.
func FromSQL[T any](value sql.Null[T]) Maybe[T] {
	return Maybe[T]{value: value.V, valid: value.Valid}
}

// Map transforms a present value, leaving an absent value absent.
func (m Maybe[T]) Map[U any](transform func(T) U) Maybe[U] {
	if !m.valid {
		return None[U]()
	}
	return Some(transform(m.value))
}

func (m Maybe[T]) IsValid() bool {
	return m.valid
}

func (m Maybe[T]) Value() T {
	return m.value
}

func (m Maybe[T]) ValueOrDefault(defaultValue T) T {
	if m.valid {
		return m.value
	}
	return defaultValue
}
