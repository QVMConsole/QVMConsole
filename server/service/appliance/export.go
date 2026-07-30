package appliance

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuildOVF 生成偏向通用兼容的 OVF 1.0 描述。
func BuildOVF(config ExportConfig) ([]byte, error) {
	if config.Name == "" || len(config.Disks) == 0 {
		return nil, fmt.Errorf("生成 OVF 时缺少虚拟机名称或磁盘")
	}
	if config.VCPU <= 0 {
		config.VCPU = 2
	}
	if config.RAMMB <= 0 {
		config.RAMMB = 2048
	}
	escape := func(value string) string {
		var b strings.Builder
		_ = xml.EscapeText(&b, []byte(value))
		return b.String()
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1" xmlns:ovf="http://schemas.dmtf.org/ovf/envelope/1" xmlns:rasd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_ResourceAllocationSettingData" xmlns:vssd="http://schemas.dmtf.org/wbem/wscim/1/cim-schema/2/CIM_VirtualSystemSettingData" xmlns:qvm="urn:qvmconsole:ovf:1">`+"\n")
	b.WriteString("  <References>\n")
	for i, disk := range config.Disks {
		fmt.Fprintf(&b, "    <File ovf:id=\"file%d\" ovf:href=\"%s\" ovf:size=\"%d\"/>\n", i+1, escape(disk.FileName), disk.FileSize)
	}
	b.WriteString("  </References>\n")
	b.WriteString("  <DiskSection>\n    <Info>虚拟磁盘</Info>\n")
	for i, disk := range config.Disks {
		fmt.Fprintf(&b, "    <Disk ovf:diskId=\"disk%d\" ovf:fileRef=\"file%d\" ovf:capacity=\"%d\" ovf:format=\"http://www.vmware.com/interfaces/specifications/vmdk.html#streamOptimized\"/>\n", i+1, i+1, disk.CapacityBytes)
	}
	b.WriteString("  </DiskSection>\n")
	if len(config.Networks) > 0 {
		b.WriteString("  <NetworkSection>\n    <Info>逻辑网络</Info>\n")
		seen := map[string]bool{}
		for _, network := range config.Networks {
			name := firstNonEmpty(network.Name, "默认网络")
			if seen[name] {
				continue
			}
			seen[name] = true
			fmt.Fprintf(&b, "    <Network ovf:name=\"%s\"><Description>导入时映射到目标平台网络</Description></Network>\n", escape(name))
		}
		b.WriteString("  </NetworkSection>\n")
	}
	fmt.Fprintf(&b, "  <VirtualSystem ovf:id=\"%s\" qvm:architecture=\"%s\" qvm:bootType=\"%s\" qvm:machineType=\"%s\">\n", escape(config.Name), escape(config.Architecture), escape(config.BootType), escape(config.MachineType))
	fmt.Fprintf(&b, "    <Info>虚拟机 %s</Info>\n    <Name>%s</Name>\n", escape(config.Name), escape(config.Name))
	fmt.Fprintf(&b, "    <OperatingSystemSection ovf:id=\"0\"><Info>操作系统</Info><Description>%s</Description></OperatingSystemSection>\n", escape(config.OSType))
	b.WriteString("    <VirtualHardwareSection>\n      <Info>虚拟硬件</Info>\n      <System><vssd:ElementName>虚拟硬件系列</vssd:ElementName><vssd:InstanceID>0</vssd:InstanceID><vssd:VirtualSystemIdentifier>")
	b.WriteString(escape(config.Name))
	b.WriteString("</vssd:VirtualSystemIdentifier><vssd:VirtualSystemType>qemu</vssd:VirtualSystemType></System>\n")
	fmt.Fprintf(&b, "      <Item><rasd:ElementName>%d 个虚拟 CPU</rasd:ElementName><rasd:InstanceID>1</rasd:InstanceID><rasd:ResourceType>3</rasd:ResourceType><rasd:VirtualQuantity>%d</rasd:VirtualQuantity></Item>\n", config.VCPU, config.VCPU)
	fmt.Fprintf(&b, "      <Item><rasd:AllocationUnits>byte * 2^20</rasd:AllocationUnits><rasd:ElementName>%d MB 内存</rasd:ElementName><rasd:InstanceID>2</rasd:InstanceID><rasd:ResourceType>4</rasd:ResourceType><rasd:VirtualQuantity>%d</rasd:VirtualQuantity></Item>\n", config.RAMMB, config.RAMMB)
	controllerByBus := make(map[string]int)
	controllerOrder := make([]string, 0, 4)
	for _, disk := range config.Disks {
		bus := normalizeExportDiskBus(disk.Bus)
		if _, exists := controllerByBus[bus]; !exists {
			controllerByBus[bus] = 3 + len(controllerOrder)
			controllerOrder = append(controllerOrder, bus)
		}
	}
	for _, bus := range controllerOrder {
		id := controllerByBus[bus]
		name, subtype, resourceType := exportControllerSpec(bus)
		fmt.Fprintf(&b, "      <Item><rasd:ElementName>%s</rasd:ElementName><rasd:InstanceID>%d</rasd:InstanceID><rasd:ResourceSubType>%s</rasd:ResourceSubType><rasd:ResourceType>%s</rasd:ResourceType></Item>\n", name, id, subtype, resourceType)
	}
	for i, disk := range config.Disks {
		bus := normalizeExportDiskBus(disk.Bus)
		fmt.Fprintf(&b, "      <Item qvm:bus=\"%s\"><rasd:ElementName>磁盘 %d</rasd:ElementName><rasd:HostResource>ovf:/disk/disk%d</rasd:HostResource><rasd:InstanceID>%d</rasd:InstanceID><rasd:Parent>%d</rasd:Parent><rasd:ResourceType>17</rasd:ResourceType></Item>\n", bus, i+1, i+1, 10+i, controllerByBus[bus])
	}
	for i, network := range config.Networks {
		fmt.Fprintf(&b, "      <Item><rasd:Connection>%s</rasd:Connection><rasd:ElementName>网卡 %d</rasd:ElementName><rasd:InstanceID>%d</rasd:InstanceID><rasd:ResourceSubType>%s</rasd:ResourceSubType><rasd:ResourceType>10</rasd:ResourceType></Item>\n", escape(firstNonEmpty(network.Name, "默认网络")), i+1, 100+i, escape(exportNICSubtype(network.Model)))
	}
	b.WriteString("    </VirtualHardwareSection>\n  </VirtualSystem>\n</Envelope>\n")
	return []byte(b.String()), nil
}

func normalizeExportDiskBus(bus string) string {
	switch strings.ToLower(strings.TrimSpace(bus)) {
	case "ide":
		return "ide"
	case "sata":
		return "sata"
	case "scsi":
		return "scsi"
	default:
		return "virtio"
	}
}

func exportControllerSpec(bus string) (name, subtype, resourceType string) {
	switch bus {
	case "ide":
		return "IDE 控制器", "PIIX4", "5"
	case "sata":
		return "SATA 控制器", "AHCI", "20"
	case "scsi":
		return "SCSI 控制器", "lsilogic", "6"
	default:
		return "VirtIO 块设备控制器", "virtio", "20"
	}
}

// CreateOVA 将描述、清单与磁盘写入单文件 OVA。
func CreateOVA(ctx context.Context, outputPath, descriptorName string, ovf []byte, disks []ExportDisk) error {
	manifestName := strings.TrimSuffix(descriptorName, filepath.Ext(descriptorName)) + ".mf"
	manifest, err := buildManifest(ctx, descriptorName, ovf, disks)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(out)
	writeBytes := func(name string, data []byte) error {
		header := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: time.Now()}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		_, err := tw.Write(data)
		return err
	}
	if err := writeBytes(descriptorName, ovf); err != nil {
		_ = tw.Close()
		_ = out.Close()
		return err
	}
	for _, disk := range disks {
		if err := writeFileToTar(ctx, tw, disk.FileName, disk.FilePath); err != nil {
			_ = tw.Close()
			_ = out.Close()
			return err
		}
	}
	if err := writeBytes(manifestName, manifest); err != nil {
		_ = tw.Close()
		_ = out.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func writeFileToTar(ctx context.Context, tw *tar.Writer, name, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	header := &tar.Header{Name: name, Mode: 0o600, Size: info.Size(), ModTime: info.ModTime()}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = copyWithContext(ctx, tw, file)
	return err
}

func buildManifest(ctx context.Context, descriptorName string, ovf []byte, disks []ExportDisk) ([]byte, error) {
	var b strings.Builder
	h := sha256.Sum256(ovf)
	fmt.Fprintf(&b, "SHA256(%s)= %s\n", descriptorName, hex.EncodeToString(h[:]))
	for _, disk := range disks {
		file, err := os.Open(disk.FilePath)
		if err != nil {
			return nil, err
		}
		hash := sha256.New()
		_, copyErr := copyWithContext(ctx, hash, file)
		_ = file.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		fmt.Fprintf(&b, "SHA256(%s)= %s\n", disk.FileName, hex.EncodeToString(hash.Sum(nil)))
	}
	return []byte(b.String()), nil
}

func exportNICSubtype(model string) string {
	switch strings.ToLower(model) {
	case "e1000e":
		return "E1000E"
	case "e1000":
		return "E1000"
	default:
		return "virtio"
	}
}
