package template

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"kvm_console/config"
)

// ListTemplates 列出所有可用模板
func ListTemplates() ([]TemplateInfo, error) {
	tree, err := buildTemplateTreeData()
	if err != nil {
		return nil, err
	}
	return tree.templates, nil
}

func buildTemplateTreeData() (*templateTreeData, error) {
	templateDir := config.GlobalConfig.TemplateDir
	files, err := filepath.Glob(filepath.Join(templateDir, "*.qcow2"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	tree := &templateTreeData{
		byName:   make(map[string]TemplateInfo),
		byNodeID: make(map[string]TemplateInfo),
		children: make(map[string][]string),
		vmByNode: make(map[string][]TemplateRelatedVM),
	}
	templates := make([]TemplateInfo, 0, len(files))

	for _, filePath := range files {
		name := strings.TrimSuffix(filepath.Base(filePath), ".qcow2")
		meta := loadTemplateMeta(filePath)
		hasMeta := meta != nil
		normalized := normalizeLoadedTemplateMeta(name, filePath, meta, hasMeta)
		if hasMeta && (normalizeTemplateBootType(meta.BootType) != normalized.BootType ||
			strings.TrimSpace(meta.Category) != normalized.Category ||
			meta.BootVerified != normalized.BootVerified) {
			if err := saveTemplateMeta(filePath, &normalized); err != nil {
				return nil, err
			}
		}
		tpl := TemplateInfo{
			Name:               name,
			Path:               filePath,
			Type:               normalized.Type,
			Category:           normalized.Category,
			BootType:           normalized.BootType,
			NVRAMPath:          normalized.NVRAMPath,
			RootPassword:       normalized.RootPassword,
			TemplateUser:       normalized.TemplateUser,
			CloudInitMode:      normalized.CloudInitMode,
			PostBootCommand:    normalized.PostBootCommand,
			PostBootBlocking:   normalized.PostBootBlocking,
			DefaultConfig:      normalized.DefaultConfig,
			HasMeta:            hasMeta,
			Exported:           HasExportedTemplate(name),
			TemplateUID:        normalized.TemplateUID,
			NodeID:             normalized.NodeID,
			ParentNodeID:       normalized.ParentNodeID,
			RootNodeID:         normalized.RootNodeID,
			AdminName:          normalized.AdminName,
			DisplayName:        normalized.DisplayName,
			CloneVisible:       normalized.CloneVisible,
			Disabled:           normalized.Disabled,
			CreatedFromVM:      normalized.CreatedFromVM,
			CreatedAt:          normalized.CreatedAt,
			DiskCompressed:     normalized.DiskCompressed,
			SourceTransferMode: normalized.SourceTransferMode,
			MD5:                normalized.MD5,
			SHA256:             normalized.SHA256,
			FileSize:           normalized.FileSize,
			LinuxInitStatus:    normalized.LinuxInitStatus,
			LinuxInitChecked:   normalized.LinuxInitChecked,
			LinuxInitError:     normalized.LinuxInitError,
		}
		if tpl.Exported {
			tpl.ExportPath = getTemplateDownloadPath(GetTemplateExportFileName(name))
		}
		templates = append(templates, tpl)
	}

	fillTemplateInfoSizesBatch(templates)
	for _, tpl := range templates {
		tree.byName[tpl.Name] = tpl
		tree.byNodeID[tpl.NodeID] = tpl
	}

	for _, tpl := range tree.byName {
		parentID := tpl.ParentNodeID
		if parentID != "" {
			if _, ok := tree.byNodeID[parentID]; ok {
				tree.children[parentID] = append(tree.children[parentID], tpl.NodeID)
			}
		}
	}
	for parentID := range tree.children {
		sort.Slice(tree.children[parentID], func(i, j int) bool {
			left := tree.byNodeID[tree.children[parentID][i]]
			right := tree.byNodeID[tree.children[parentID][j]]
			return left.AdminName < right.AdminName
		})
	}

	if err := attachTemplateVMCounts(tree); err != nil {
		return nil, err
	}

	var roots []string
	for _, tpl := range tree.byName {
		if tpl.ParentNodeID == "" || tree.byNodeID[tpl.ParentNodeID].NodeID == "" {
			roots = append(roots, tpl.NodeID)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		left := tree.byNodeID[roots[i]]
		right := tree.byNodeID[roots[j]]
		return left.AdminName < right.AdminName
	})

	var computeTotal func(string) int
	computeTotal = func(nodeID string) int {
		tpl := tree.byNodeID[nodeID]
		tpl.ChildrenCount = len(tree.children[nodeID])
		tpl.HasChildren = tpl.ChildrenCount > 0
		tpl.DirectVMCount = countLinkedVMs(tree.vmByNode[nodeID])
		total := tpl.DirectVMCount
		for _, childID := range tree.children[nodeID] {
			total += computeTotal(childID)
		}
		tpl.TreeVMCount = total
		tree.byName[tpl.Name] = tpl
		tree.byNodeID[tpl.NodeID] = tpl
		return total
	}
	for _, rootID := range roots {
		computeTotal(rootID)
	}

	var ordered []TemplateInfo
	var walkOrder func(string, int)
	walkOrder = func(nodeID string, level int) {
		tpl := tree.byNodeID[nodeID]
		tpl.Level = level
		tpl.IsRoot = tpl.ParentNodeID == "" || tree.byNodeID[tpl.ParentNodeID].NodeID == ""
		tree.byName[tpl.Name] = tpl
		tree.byNodeID[tpl.NodeID] = tpl
		ordered = append(ordered, tpl)
		for _, childID := range tree.children[nodeID] {
			walkOrder(childID, level+1)
		}
	}
	for _, rootID := range roots {
		walkOrder(rootID, 0)
	}
	tree.templates = ordered
	return tree, nil
}

func attachTemplateVMCounts(tree *templateTreeData) error {
	vmSources, err := listVMTemplateSources()
	if err != nil {
		return err
	}
	for _, vm := range vmSources {
		tplName := strings.TrimSpace(vm.Template)
		if tplName == "" {
			continue
		}
		tpl, ok := tree.byName[tplName]
		if !ok {
			continue
		}
		nodeID := tpl.NodeID
		if vm.NodeID != "" {
			if sourceTpl, exists := tree.byNodeID[vm.NodeID]; exists {
				nodeID = sourceTpl.NodeID
				tplName = sourceTpl.Name
			}
		}
		tree.vmByNode[nodeID] = append(tree.vmByNode[nodeID], TemplateRelatedVM{
			Name:      vm.Name,
			Template:  tplName,
			NodeID:    nodeID,
			CloneMode: vm.CloneMode,
		})
	}
	for nodeID := range tree.vmByNode {
		sort.Slice(tree.vmByNode[nodeID], func(i, j int) bool {
			return tree.vmByNode[nodeID][i].Name < tree.vmByNode[nodeID][j].Name
		})
	}
	return nil
}

func countLinkedVMs(vms []TemplateRelatedVM) int {
	n := 0
	for _, vm := range vms {
		if vm.CloneMode != "full" {
			n++
		}
	}
	return n
}

func collectTemplateSubtree(tree *templateTreeData, nodeID string) []TemplateInfo {
	var templates []TemplateInfo
	if tpl, ok := tree.byNodeID[nodeID]; ok {
		templates = append(templates, tpl)
	}
	for _, childID := range tree.children[nodeID] {
		templates = append(templates, collectTemplateSubtree(tree, childID)...)
	}
	return templates
}

func directChildTemplates(tree *templateTreeData, nodeID string) []TemplateInfo {
	children := make([]TemplateInfo, 0, len(tree.children[nodeID]))
	for _, childID := range tree.children[nodeID] {
		if child, ok := tree.byNodeID[childID]; ok {
			children = append(children, child)
		}
	}
	return children
}

func filterLinkedVMs(vms []TemplateRelatedVM) []TemplateRelatedVM {
	filtered := make([]TemplateRelatedVM, 0, len(vms))
	for _, vm := range vms {
		if vm.CloneMode == "full" {
			continue
		}
		filtered = append(filtered, vm)
	}
	return filtered
}

func collectTemplateSubtreeVMs(tree *templateTreeData, nodeID string) []TemplateRelatedVM {
	vms := make([]TemplateRelatedVM, 0)
	for _, vm := range tree.vmByNode[nodeID] {
		if vm.CloneMode == "full" {
			continue
		}
		vms = append(vms, vm)
	}
	for _, childID := range tree.children[nodeID] {
		vms = append(vms, collectTemplateSubtreeVMs(tree, childID)...)
	}
	sort.Slice(vms, func(i, j int) bool {
		return vms[i].Name < vms[j].Name
	})
	return vms
}

// isVMStateShutoff checks if a VM state string indicates the VM is shut off.
func isVMStateShutoff(state string) bool {
	normalized := strings.ToLower(strings.TrimSpace(state))
	return normalized == "shut off" || normalized == "shutoff"
}

func getTemplateDownloadPath(fileName string) string {
	return "/api/template/download/" + url.PathEscape(fileName)
}
