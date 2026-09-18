package snapshot

import (
	"regexp"
)

var snapshotNameRegexp = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)

const (
	appArmorManagedBlockBegin       = "# BEGIN kvm_console managed storage access"
	appArmorManagedBlockEnd         = "# END kvm_console managed storage access"
	appArmorVirtAAHelperLocalPath   = "/etc/apparmor.d/local/usr.lib.libvirt.virt-aa-helper"
	appArmorLibvirtQemuStoragePath  = "/etc/apparmor.d/abstractions/libvirt-qemu.d/kvm-console-storage"
	appArmorVirtAAHelperProfilePath = "/etc/apparmor.d/usr.lib.libvirt.virt-aa-helper"
)

// SnapshotInfo 快照信息
type SnapshotInfo struct {
	Name        string `json:"name"`
	CreatedAt   string `json:"created_at"`
	State       string `json:"state"`
	Description string `json:"description"`
	IsCurrent   bool   `json:"is_current"`
	Location    string `json:"location"` // internal / external
	Children    int    `json:"children"`
	Descendants int    `json:"descendants"`
}

// SnapshotQuotaInfo 快照配额信息。
type SnapshotQuotaInfo struct {
	Scope              string `json:"scope"`
	UsedSnapshots      int    `json:"used_snapshots"`
	MaxSnapshots       int    `json:"max_snapshots"`
	RemainingSnapshots int    `json:"remaining_snapshots"`
}

type snapshotXMLDescription struct {
	Description string `xml:"description"`
}

type snapshotInfoOutput struct {
	State       string
	Location    string
	Children    int
	Descendants int
}

type vmDiskSource struct {
	Target string
	Source string
}

type externalSnapshotDiskXML struct {
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
}

type externalSnapshotDomainDiskXML struct {
	Source struct {
		File string `xml:"file,attr"`
	} `xml:"source"`
	Target struct {
		Dev string `xml:"dev,attr"`
	} `xml:"target"`
}

type externalSnapshotXML struct {
	Disks  []externalSnapshotDiskXML `xml:"disks>disk"`
	Domain struct {
		Devices struct {
			Disks []externalSnapshotDomainDiskXML `xml:"disk"`
		} `xml:"devices"`
	} `xml:"domain"`
}
