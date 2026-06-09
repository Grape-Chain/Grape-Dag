package common

import "reflect"

type IEvent interface {
	Event(evt reflect.Type)
}
