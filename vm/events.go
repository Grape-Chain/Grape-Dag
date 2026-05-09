package vm

var logCallbacks []func([]Log)

func RegisterLogCallback(callback func([]Log)) {
	logCallbacks = append(logCallbacks, callback)
}

func TriggerCallbacks(logs []Log) {
	for _, callback := range logCallbacks {
		go callback(logs)
	}
}
