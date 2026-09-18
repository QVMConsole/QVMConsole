package vnc

// deps.go — 声明 Hook 变量，避免反向 import service 根包。

var (
	// VM lifecycle hooks
	HookStartVM        func(name string) error
	HookDetectVMOSType func(templateName, xmlStr string) string
)
