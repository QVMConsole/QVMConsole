package guest_automation

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"kvm_console/service/guest_agent"
	"kvm_console/service/libvirt_rpc"
	"kvm_console/service/storage/disk"
)

var (
	devicePattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	mountPattern  = regexp.MustCompile(`^/[a-zA-Z0-9._/-]+$`)
)

// GuestMountConfig 是所有磁盘入口复用的来宾挂载配置。
type GuestMountConfig = disk.GuestMountConfig

// DiskOperationParams 描述异步磁盘操作。
type DiskOperationParams struct {
	Action       string           `json:"action"`
	VMName       string           `json:"vm_name"`
	Device       string           `json:"device,omitempty"`
	SizeGB       int              `json:"size_gb,omitempty"`
	Format       string           `json:"format,omitempty"`
	Bus          string           `json:"bus,omitempty"`
	DiskPath     string           `json:"disk_path,omitempty"`
	StorageDir   string           `json:"storage_dir,omitempty"`
	GuestType    string           `json:"guest_type,omitempty"`
	ExistingDisk bool             `json:"existing_disk,omitempty"`
	GuestMount   GuestMountConfig `json:"guest_mount,omitempty"`
	CreatedBy    string           `json:"created_by,omitempty"`
}

type StageResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type VolumeResult struct {
	Device     string `json:"device"`
	Filesystem string `json:"filesystem,omitempty"`
	Target     string `json:"target,omitempty"`
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
}

// OperationResult 明确区分宿主机阶段和来宾阶段，支持展示部分成功。
type OperationResult struct {
	HostStage  StageResult    `json:"host_stage"`
	GuestStage StageResult    `json:"guest_stage"`
	Device     string         `json:"device,omitempty"`
	Volumes    []VolumeResult `json:"volumes,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
	Retryable  bool           `json:"retryable"`
}

func ParseDiskOperationParams(raw string) (*DiskOperationParams, error) {
	var params DiskOperationParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return nil, err
	}
	return &params, nil
}

func ValidateGuestMount(config *GuestMountConfig, guestType string) error {
	if config == nil || !config.Enabled {
		return nil
	}
	guestType = strings.ToLower(strings.TrimSpace(guestType))
	if guestType != "linux" && guestType != "windows" {
		return fmt.Errorf("自动挂载仅适用于 Linux 和 Windows 虚拟机")
	}
	if guestType == "linux" {
		filesystem := strings.ToLower(strings.TrimSpace(config.Filesystem))
		if filesystem == "" {
			filesystem = "ext4"
		}
		if filesystem != "ext4" && filesystem != "xfs" && filesystem != "btrfs" {
			return fmt.Errorf("Linux 文件系统仅支持 ext4、XFS 和 Btrfs")
		}
		mountPoint := strings.TrimSpace(config.MountPoint)
		if mountPoint == "" {
			mountPoint = "/data"
		}
		cleaned := path.Clean(mountPoint)
		if !mountPattern.MatchString(cleaned) || cleaned == "/" || strings.HasPrefix(cleaned, "/boot") || strings.HasPrefix(cleaned, "/etc") {
			return fmt.Errorf("挂载目录格式无效")
		}
		config.Filesystem = filesystem
		config.MountPoint = cleaned
	} else {
		letter := strings.ToUpper(strings.TrimSpace(config.DriveLetter))
		letter = strings.TrimSuffix(letter, ":")
		if letter != "" && (len(letter) != 1 || letter[0] < 'D' || letter[0] > 'Z') {
			return fmt.Errorf("Windows 盘符需为 D 到 Z")
		}
		config.DriveLetter = letter
	}
	return nil
}

// Preflight 在宿主机发生变更前检查运行状态、QGA 和必需命令能力。
func Preflight(ctx context.Context, vmName, guestType string, mount *GuestMountConfig, requireGrow bool) error {
	state, err := libvirt_rpc.GetDomainStateRPC(vmName)
	if err != nil {
		return err
	}
	if state != "running" {
		return fmt.Errorf("来宾自动化要求虚拟机处于运行状态")
	}
	if err := ValidateGuestMount(mount, guestType); err != nil {
		return err
	}
	client := guest_agent.NewClient(vmName)
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("QEMU Guest Agent 未连接: %w", err)
	}
	if !client.Supports(ctx, "guest-exec") || !client.Supports(ctx, "guest-exec-status") {
		return fmt.Errorf("QEMU Guest Agent 未启用来宾命令执行能力")
	}
	if requireGrow && strings.ToLower(strings.TrimSpace(guestType)) != "linux" {
		return fmt.Errorf("自动扩容系统分区仅适用于 Linux 虚拟机")
	}
	return nil
}

// RunDiskOperation 执行宿主机磁盘阶段及可选来宾阶段。
func RunDiskOperation(ctx context.Context, params *DiskOperationParams, progress func(int, string)) (*OperationResult, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	result := &OperationResult{
		HostStage: StageResult{Status: "pending"}, GuestStage: StageResult{Status: "skipped"},
	}
	err := guest_agent.WithVMOperationLock(params.VMName, func() error {
		if params.GuestMount.Enabled {
			requireGrow := params.Action == "resize" || params.Action == "guest_grow"
			mountConfig := &params.GuestMount
			if requireGrow {
				mountConfig = nil
			}
			if err := Preflight(ctx, params.VMName, params.GuestType, mountConfig, requireGrow); err != nil {
				result.HostStage = StageResult{Status: "skipped", Message: "来宾预检未通过，宿主机未变更"}
				result.GuestStage = StageResult{Status: "failed", Message: err.Error()}
				result.Retryable = true
				return err
			}
		}
		switch params.Action {
		case "resize":
			progress(10, "正在扩容宿主机磁盘...")
			if err := disk.ResizeDisk(params.VMName, params.Device, params.SizeGB); err != nil {
				result.HostStage = StageResult{Status: "failed", Message: err.Error()}
				return err
			}
			result.Device = params.Device
			result.HostStage = StageResult{Status: "success", Message: "宿主机磁盘容量已扩大"}
			if params.GuestMount.Enabled {
				return runGrowStage(ctx, params, result, progress)
			}
		case "guest_grow":
			result.Device = params.Device
			result.HostStage = StageResult{Status: "skipped", Message: "重试仅执行来宾系统阶段"}
			return runGrowStage(ctx, params, result, progress)
		case "add":
			progress(10, "正在创建并连接磁盘...")
			var dev string
			var err error
			if strings.TrimSpace(params.StorageDir) != "" {
				dev, err = disk.AddDiskWithBusInDir(params.VMName, params.SizeGB, params.Format, params.Bus, params.StorageDir)
			} else {
				dev, err = disk.AddDiskWithBus(params.VMName, params.SizeGB, params.Format, params.Bus)
			}
			if err != nil {
				result.HostStage = StageResult{Status: "failed", Message: err.Error()}
				return err
			}
			params.Device, result.Device = dev, dev
			result.HostStage = StageResult{Status: "success", Message: "磁盘已连接到虚拟机"}
			if params.GuestMount.Enabled {
				return runMountStage(ctx, params, result, progress, true)
			}
		case "attach":
			progress(10, "正在连接已有磁盘...")
			dev, err := disk.AttachExistingDisk(params.VMName, params.DiskPath, params.Bus)
			if err != nil {
				result.HostStage = StageResult{Status: "failed", Message: err.Error()}
				return err
			}
			params.Device, result.Device = dev, dev
			result.HostStage = StageResult{Status: "success", Message: "已有磁盘已连接到虚拟机"}
			if params.GuestMount.Enabled {
				return runMountStage(ctx, params, result, progress, false)
			}
		case "guest_mount":
			result.Device = params.Device
			result.HostStage = StageResult{Status: "skipped", Message: "重试仅执行来宾系统阶段"}
			return runMountStage(ctx, params, result, progress, !params.ExistingDisk)
		default:
			return fmt.Errorf("未知的磁盘操作: %s", params.Action)
		}
		return nil
	})
	return result, err
}

func runGrowStage(ctx context.Context, params *DiskOperationParams, result *OperationResult, progress func(int, string)) error {
	progress(55, "正在识别并扩展 Linux 系统分区...")
	guestDevice, err := waitGuestDevice(ctx, params.VMName, params.Device, 30*time.Second)
	if err != nil {
		return markGuestFailure(result, err)
	}
	client := guest_agent.NewClient(params.VMName)
	execResult, err := client.Execute(ctx, "/bin/sh", []string{"-c", linuxGrowScript(guestDevice)}, guest_agent.DiskTimeout)
	if err != nil {
		return markGuestFailure(result, err)
	}
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		if message == "" {
			message = strings.TrimSpace(execResult.Stdout)
		}
		return markGuestFailure(result, fmt.Errorf("系统分区扩容失败: %s", message))
	}
	result.GuestStage = StageResult{Status: "success", Message: strings.TrimSpace(execResult.Stdout)}
	progress(100, "系统分区和文件系统扩容完成")
	return nil
}

func runMountStage(ctx context.Context, params *DiskOperationParams, result *OperationResult, progress func(int, string), newBlank bool) error {
	progress(55, "正在识别来宾磁盘并配置挂载...")
	guestDevice, err := waitGuestDevice(ctx, params.VMName, params.Device, 30*time.Second)
	if err != nil {
		return markGuestFailure(result, err)
	}
	client := guest_agent.NewClient(params.VMName)
	var executable string
	var args []string
	if strings.EqualFold(params.GuestType, "windows") {
		executable = "powershell.exe"
		args = []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", windowsMountScript(guestDevice, params.GuestMount.DriveLetter, newBlank)}
	} else {
		executable = "/bin/sh"
		args = []string{"-c", linuxMountScript(guestDevice, params.GuestMount, newBlank)}
	}
	execResult, err := client.Execute(ctx, executable, args, guest_agent.DiskTimeout)
	if err != nil {
		return markGuestFailure(result, err)
	}
	parseVolumeOutput(execResult.Stdout, result)
	if execResult.ExitCode != 0 {
		message := strings.TrimSpace(execResult.Stderr)
		if message == "" {
			message = strings.TrimSpace(execResult.Stdout)
		}
		return markGuestFailure(result, fmt.Errorf("来宾磁盘挂载失败: %s", message))
	}
	mountedVolumes := 0
	for _, volume := range result.Volumes {
		if volume.Status == "mounted" {
			mountedVolumes++
		}
	}
	if len(result.Volumes) == 0 {
		return markGuestFailure(result, fmt.Errorf("磁盘中未发现可处理的数据卷；新空盘请明确选择初始化后挂载"))
	}
	if newBlank && mountedVolumes == 0 {
		return markGuestFailure(result, fmt.Errorf("新磁盘已初始化，但未确认任何数据卷挂载成功"))
	}
	message := "来宾磁盘挂载配置完成"
	if mountedVolumes == 0 {
		message = "未发现需要新增挂载的数据卷"
	}
	result.GuestStage = StageResult{Status: "success", Message: message}
	progress(100, "来宾磁盘挂载完成")
	return nil
}

func markGuestFailure(result *OperationResult, err error) error {
	result.GuestStage = StageResult{Status: "failed", Message: err.Error()}
	result.Retryable = true
	return err
}

func waitGuestDevice(ctx context.Context, vmName, hostDevice string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		disks, err := disk.ListDisks(vmName)
		if err == nil {
			for _, item := range disks {
				if item.Device == hostDevice && item.GuestMappingStatus == "mapped" && item.GuestDevice != "" {
					return item.GuestDevice, nil
				}
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("等待来宾识别磁盘 %s 超时", hostDevice)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func linuxGrowScript(device string) string {
	dev := shellQuote(device)
	return `set -eu
dev=` + dev + `
[ -b "$dev" ] || { echo "目标磁盘不存在" >&2; exit 10; }
root_mnt=/
root_src=$(findmnt -nro SOURCE "$root_mnt" | sed 's/\[.*//')
root_fs=$(findmnt -nro FSTYPE "$root_mnt" | tr '[:upper:]' '[:lower:]')
case "$root_fs" in ext4|xfs|btrfs) ;; *) echo "根文件系统仅支持 ext4、xfs、btrfs" >&2; exit 11;; esac
if [ "$root_fs" = btrfs ] && [ "$(btrfs filesystem show / | grep -c 'devid')" -ne 1 ]; then echo "多设备 Btrfs 不执行自动扩容" >&2; exit 12; fi
dev_real=$(readlink -f "$dev")
root_real=$(readlink -f "$root_src")
if ! lsblk -slnpo NAME "$root_real" | grep -Fxq "$dev_real"; then echo "所选磁盘不是根文件系统所在磁盘" >&2; exit 13; fi
part=$(lsblk -slnpo NAME,TYPE "$root_real" | awk -v d="$dev_real" '$2=="part"{print $1}' | tail -1)
if [ -n "$part" ]; then
  part=$(readlink -f "$part")
  part_start=$(lsblk -bnro START "$part" | head -1)
  [ -n "$part_start" ] || { echo "读取根分区起始位置失败" >&2; exit 14; }
  blocker=$(lsblk -bnrpo NAME,TYPE,START "$dev_real" | awk -v p="$part" -v s="$part_start" '$2=="part" && $1!=p && $3>s {print $1; exit}')
  [ -z "$blocker" ] || { echo "根分区后方仍有分区 $blocker" >&2; exit 14; }
  command -v growpart >/dev/null || { echo "缺少 growpart" >&2; exit 15; }
  num=$(lsblk -nro PARTN "$part")
  if ! grow_output=$(growpart "$dev_real" "$num" 2>&1); then
    case "$grow_output" in
      NOCHANGE:*) ;;
      *) printf '%s\n' "$grow_output" >&2; exit 15 ;;
    esac
  fi
  printf '%s\n' "$grow_output"
  command -v partprobe >/dev/null && partprobe "$dev_real" || true
  command -v udevadm >/dev/null && udevadm settle || true
fi
if [ "$(lsblk -dnro TYPE "$root_real")" = "lvm" ]; then
  for cmd in pvs lvs pvresize lvextend; do command -v "$cmd" >/dev/null || { echo "缺少 $cmd" >&2; exit 16; }; done
  vg=$(lvs --noheadings -o vg_name "$root_src" | xargs)
  pv=$(pvs --noheadings -o pv_name,vg_name | awk -v vg="$vg" '$2==vg{print $1; exit}')
  [ -n "$pv" ] || { echo "未找到根逻辑卷对应的 PV" >&2; exit 16; }
  if ! lsblk -slnpo NAME "$(readlink -f "$pv")" | grep -Fxq "$dev_real"; then echo "根逻辑卷 PV 不属于所选磁盘" >&2; exit 17; fi
  pvresize "$pv"
  lvextend -l +100%FREE "$root_src"
fi
case "$root_fs" in
  ext4) resize2fs "$root_real" ;;
  xfs) xfs_growfs / ;;
  btrfs) btrfs filesystem resize max / ;;
esac
echo "根文件系统 $root_fs 已扩容"`
}

func linuxMountScript(device string, config GuestMountConfig, newBlank bool) string {
	newValue := "0"
	if newBlank {
		newValue = "1"
	}
	return `set -eu
dev=` + shellQuote(device) + `
fstype=` + shellQuote(config.Filesystem) + `
base=` + shellQuote(config.MountPoint) + `
new_blank=` + newValue + `
[ -b "$dev" ] || { echo "目标磁盘不存在" >&2; exit 20; }
for cmd in lsblk blkid findmnt mount; do command -v "$cmd" >/dev/null || { echo "缺少 $cmd" >&2; exit 21; }; done
already_initialized=0
if [ "$new_blank" = 1 ]; then
  pre_mounted=0
  for candidate in $(lsblk -nrpo NAME "$dev"); do
    candidate_target=$(findmnt -nr -S "$candidate" -o TARGET 2>/dev/null | head -1 || true)
    [ -n "$candidate_target" ] || continue
    pre_mounted=$((pre_mounted+1))
    candidate_fs=$(blkid -s TYPE -o value "$candidate" 2>/dev/null || true)
    if [ "$candidate_target" != "$base" ] || [ "$candidate_fs" != "$fstype" ]; then
      echo "磁盘卷 $candidate 已挂载到 $candidate_target，文件系统为 $candidate_fs，未执行重新初始化" >&2
      exit 26
    fi
  done
  [ "$pre_mounted" -le 1 ] || { echo "磁盘包含多个已挂载卷，未执行重新初始化" >&2; exit 26; }
  if [ "$pre_mounted" -eq 1 ]; then already_initialized=1; new_blank=0; fi
fi
if [ "$new_blank" = 1 ]; then
  command -v sgdisk >/dev/null || { echo "缺少 sgdisk" >&2; exit 22; }
  sgdisk --zap-all --new=1:0:0 --typecode=1:8300 "$dev"
  command -v partprobe >/dev/null && partprobe "$dev" || true
  command -v udevadm >/dev/null && udevadm settle || true
  part=$(lsblk -lnpo NAME,TYPE "$dev" | awk '$2=="part"{print $1; exit}')
  [ -n "$part" ] || { echo "新分区尚未出现" >&2; exit 23; }
  case "$fstype" in
    ext4) mkfs.ext4 -F "$part" ;;
    xfs) mkfs.xfs -f "$part" ;;
    btrfs) mkfs.btrfs -f "$part" ;;
    *) echo "文件系统仅支持 ext4、xfs、btrfs" >&2; exit 24 ;;
  esac
fi
tmp=$(mktemp /etc/.fstab.qvm.XXXXXX)
trap 'rm -f "$tmp"' EXIT HUP INT TERM
cp /etc/fstab "/etc/fstab.qvm.$(date +%Y%m%d%H%M%S).bak"
cp /etc/fstab "$tmp"
persist_fstab() {
  persist_uuid="$1"
  persist_target="$2"
  persist_fs="$3"
  for old_uuid in $(awk -v t="$persist_target" '$1 ~ /^UUID=/ && $2==t {sub(/^UUID=/, "", $1); print $1}' "$tmp"); do
    if [ "$old_uuid" != "$persist_uuid" ] && ! blkid -U "$old_uuid" >/dev/null 2>&1; then
      clean="${tmp}.clean"
      awk -v s="UUID=$old_uuid" -v t="$persist_target" '$1!=s || $2!=t' "$tmp" > "$clean"
      mv "$clean" "$tmp"
    fi
  done
  grep -qE "^[[:space:]]*UUID=$persist_uuid[[:space:]]" "$tmp" || printf 'UUID=%s %s %s defaults,nofail 0 2\n' "$persist_uuid" "$persist_target" "$persist_fs" >> "$tmp"
}
count=0
processed=0
mounted_count=0
if ! nodes=$(lsblk -nrpo NAME "$dev"); then echo "读取磁盘卷列表失败" >&2; exit 25; fi
[ -n "$nodes" ] || { echo "磁盘未返回可处理的设备" >&2; exit 25; }
for node in $nodes; do
  fs=$(blkid -s TYPE -o value "$node" 2>/dev/null || true)
  type=$(lsblk -dnro TYPE "$node" 2>/dev/null || true)
  mounted=$(findmnt -nr -S "$node" -o TARGET 2>/dev/null | head -1 || true)
  case "$fs" in ext4|xfs|btrfs) ;; *) [ -n "$fs" ] && echo "VOLUME|$node|$fs||skipped|文件系统不在支持范围"; continue;; esac
  processed=$((processed+1))
  [ "$type" = part ] || [ "$type" = disk ] || { echo "VOLUME|$node|$fs||skipped|设备类型不适合自动挂载"; continue; }
  if [ -n "${mounted:-}" ]; then
    uuid=$(blkid -s UUID -o value "$node")
    [ -n "$uuid" ] || { echo "VOLUME|$node|$fs|$mounted|skipped|卷没有 UUID"; continue; }
    persist_fstab "$uuid" "$mounted" "$fs"
    if [ "$already_initialized" = 1 ]; then
      mounted_count=$((mounted_count+1))
      echo "VOLUME|$node|$fs|$mounted|mounted|卷已按目标配置"
    else
      echo "VOLUME|$node|$fs|$mounted|skipped|卷已挂载"
    fi
    continue
  fi
  count=$((count+1))
  target="$base"
  [ "$count" -eq 1 ] || target="${base}${count}"
  while findmnt -rn "$target" >/dev/null 2>&1 || { [ -d "$target" ] && [ -n "$(ls -A "$target" 2>/dev/null)" ]; }; do count=$((count+1)); target="${base}${count}"; done
  mkdir -p "$target"
  uuid=$(blkid -s UUID -o value "$node")
  [ -n "$uuid" ] || { echo "VOLUME|$node|$fs|$target|skipped|卷没有 UUID"; continue; }
  persist_fstab "$uuid" "$target" "$fs"
  mount -t "$fs" "$node" "$target"
  mounted_count=$((mounted_count+1))
  echo "VOLUME|$node|$fs|$target|mounted|"
done
[ "$new_blank" != 1 ] || [ "$mounted_count" -gt 0 ] || { echo "新磁盘格式化后未找到可挂载卷" >&2; exit 25; }
chmod --reference=/etc/fstab "$tmp"
chown --reference=/etc/fstab "$tmp"
mv "$tmp" /etc/fstab
trap - EXIT HUP INT TERM
echo "SUMMARY|$processed|$mounted_count"`
}

func windowsMountScript(device, preferredLetter string, newBlank bool) string {
	number := regexp.MustCompile(`(?i)physicaldrive(\d+)$`).FindStringSubmatch(device)
	diskSelector := "$null"
	if len(number) == 2 {
		diskSelector = number[1]
	}
	newValue := "$false"
	if newBlank {
		newValue = "$true"
	}
	preferred := strings.ToUpper(strings.TrimSuffix(preferredLetter, ":"))
	return `$ErrorActionPreference='Stop'; $diskNumber=` + diskSelector + `; if ($null -eq $diskNumber) { throw '来宾磁盘映射未返回 PhysicalDrive 编号' }; $disk=Get-Disk -Number $diskNumber; if (` + newValue + `) { if ($disk.PartitionStyle -ne 'RAW') { throw '新磁盘已包含分区，已停止初始化' }; Initialize-Disk -Number $diskNumber -PartitionStyle GPT; $p=New-Partition -DiskNumber $diskNumber -UseMaximumSize; Format-Volume -Partition $p -FileSystem NTFS -Confirm:$false | Out-Null }; $preferred='` + preferred + `'; $eligible=Get-Partition -DiskNumber $diskNumber | Where-Object { -not $_.IsSystem -and -not $_.IsBoot -and $_.Type -notmatch 'Recovery|System|Reserved' }; $first=$true; foreach($p in $eligible){ $v=$p | Get-Volume -ErrorAction SilentlyContinue; if($null -eq $v -or @('NTFS','ReFS') -notcontains $v.FileSystem){ Write-Output ('VOLUME|Disk'+$diskNumber+'Partition'+$p.PartitionNumber+'|'+$v.FileSystem+'||skipped|卷格式或类型不适合自动挂载'); continue }; if($p.AccessPaths.Count -gt 0 -and $p.DriveLetter){ Write-Output ('VOLUME|Disk'+$diskNumber+'Partition'+$p.PartitionNumber+'|'+$v.FileSystem+'|'+$p.DriveLetter+':|skipped|卷已有盘符'); continue }; $letter=$null; if($first -and $preferred){ if(-not (Get-Volume -DriveLetter $preferred -ErrorAction SilentlyContinue)){ $letter=$preferred } }; if(-not $letter){ $used=(Get-Volume | Where-Object DriveLetter | ForEach-Object { [string]$_.DriveLetter }); $letter=([char[]](68..90) | Where-Object { $used -notcontains [string]$_ } | Select-Object -First 1) }; if(-not $letter){ throw '没有可用盘符' }; Set-Partition -DiskNumber $diskNumber -PartitionNumber $p.PartitionNumber -NewDriveLetter $letter; Write-Output ('VOLUME|Disk'+$diskNumber+'Partition'+$p.PartitionNumber+'|'+$v.FileSystem+'|'+$letter+':|mounted|'); $first=$false }`
}

func parseVolumeOutput(output string, result *OperationResult) {
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(strings.TrimSpace(line), "|")
		if len(parts) != 6 || parts[0] != "VOLUME" {
			continue
		}
		result.Volumes = append(result.Volumes, VolumeResult{
			Device: parts[1], Filesystem: parts[2], Target: parts[3], Status: parts[4], Message: parts[5],
		})
		if parts[4] == "skipped" && parts[5] != "" {
			result.Warnings = append(result.Warnings, parts[1]+": "+parts[5])
		}
	}
}

// GuestDiskStatus 返回单块磁盘的来宾映射和文件系统状态。
func GuestDiskStatus(ctx context.Context, vmName, device string) (map[string]any, error) {
	if !devicePattern.MatchString(device) {
		return nil, fmt.Errorf("磁盘设备名格式无效")
	}
	items, err := disk.ListDisks(vmName)
	if err != nil {
		return nil, err
	}
	var selected *disk.DiskInfo
	for i := range items {
		if items[i].Device == device {
			selected = &items[i]
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("磁盘 %s 不存在", device)
	}
	filesystems, fsErr := guest_agent.NewClient(vmName).Filesystems(ctx)
	return map[string]any{"disk": selected, "filesystems": filesystems, "filesystems_error": errorText(fsErr)}, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func BuildResultJSON(result *OperationResult) string {
	encoded, _ := json.Marshal(result)
	return string(encoded)
}
