package template

import "fmt"

// GetLinuxTemplatePrepareCheck returns linked VMs in the target template subtree.
// 在位链式 VM 会直接或间接依赖当前模板的 backing，因此模板磁盘不可原地改写。
func GetLinuxTemplatePrepareCheck(templateName string) (*LinuxTemplatePrepareCheck, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	tpl, ok := tree.byName[templateName]
	if !ok {
		return nil, fmt.Errorf("模板不存在: %s", templateName)
	}

	linkedVMs := hydrateTemplateRelatedVMs(collectTemplateSubtreeVMs(tree, tpl.NodeID))
	return &LinuxTemplatePrepareCheck{
		TemplateName: tpl.Name,
		LinkedVMs:    linkedVMs,
		CanPrepare:   len(linkedVMs) == 0,
	}, nil
}

func ensureLinuxTemplateCanBePrepared(templateName string) error {
	check, err := GetLinuxTemplatePrepareCheck(templateName)
	if err != nil {
		return err
	}
	if check.CanPrepare {
		return nil
	}
	return fmt.Errorf(
		"模板链路仍关联 %d 台链式克隆虚拟机，请先在“虚拟机管理”的“更多”菜单中逐台执行“转为独立虚拟机”，待转换任务全部完成后再预处理",
		len(check.LinkedVMs),
	)
}
