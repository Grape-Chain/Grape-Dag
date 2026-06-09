package common

import "reflect"

type IEvtBus interface {
	WaitForEvent(evt reflect.Type)
}
