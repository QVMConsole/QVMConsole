package arch

// NormalizeMachineType 将界面和历史接口中的机型别名转换为 virt-install 可识别的机型值。
func NormalizeMachineType(archName, machineType string) string {
	if archName != ArchX8664 {
		return machineType
	}

	switch machineType {
	case "i440fx", "pc-i440fx":
		return "pc"
	default:
		return machineType
	}
}
