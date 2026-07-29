package guest_agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"kvm_console/logger"
	"kvm_console/service/libvirt_rpc"
)

const (
	ConnectTimeout = 10 * time.Second
	ExecuteTimeout = 120 * time.Second
	DiskTimeout    = 300 * time.Second
	maxOutputBytes = 1024 * 1024
)

// Client 是单台虚拟机的 QEMU Guest Agent 客户端。
type Client struct {
	VMName string
}

// CommandInfo 描述 QGA 报告的命令能力。
type CommandInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// AgentInfo 描述 QGA 版本和能力。
type AgentInfo struct {
	Version           string        `json:"version"`
	SupportedCommands []CommandInfo `json:"supported_commands"`
}

// OSInfo 描述来宾操作系统。
type OSInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PrettyName    string `json:"pretty-name"`
	Version       string `json:"version"`
	VersionID     string `json:"version-id"`
	KernelRelease string `json:"kernel-release"`
	KernelVersion string `json:"kernel-version"`
	Machine       string `json:"machine"`
}

// PCIDeviceAddress 是 QGA 返回的 PCI 地址。
type PCIDeviceAddress struct {
	Domain   int `json:"domain"`
	Bus      int `json:"bus"`
	Slot     int `json:"slot"`
	Function int `json:"function"`
}

// DiskAddress 描述来宾磁盘的稳定标识和控制器地址。
type DiskAddress struct {
	Serial        string            `json:"serial"`
	PCIDevice     *PCIDeviceAddress `json:"pci-controller"`
	CCWAddress    *json.RawMessage  `json:"ccw-address"`
	VirtioAddress *json.RawMessage  `json:"virtio-address"`
}

// GuestDisk 是 guest-get-disks 的精简结果。
type GuestDisk struct {
	Name         string       `json:"name"`
	Partition    bool         `json:"partition"`
	Dependencies []string     `json:"dependencies"`
	Address      *DiskAddress `json:"address"`
}

// GuestFilesystem 是 guest-get-fsinfo 的精简结果。
type GuestFilesystem struct {
	Name       string      `json:"name"`
	Mountpoint string      `json:"mountpoint"`
	Type       string      `json:"type"`
	Disk       []GuestDisk `json:"disk"`
}

// ExecResult 是 guest-exec 的完成结果。
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type commandEnvelope struct {
	Return json.RawMessage `json:"return"`
	Error  *struct {
		Class string `json:"class"`
		Desc  string `json:"desc"`
	} `json:"error,omitempty"`
}

var vmOperationLocks sync.Map

// WithVMOperationLock 保证同一虚拟机的来宾磁盘和密码操作串行执行。
func WithVMOperationLock(vmName string, fn func() error) error {
	key := strings.TrimSpace(vmName)
	value, _ := vmOperationLocks.LoadOrStore(key, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

// NewClient 创建指定虚拟机的 QGA 客户端。
func NewClient(vmName string) *Client {
	return &Client{VMName: strings.TrimSpace(vmName)}
}

func durationSeconds(timeout time.Duration) int32 {
	if timeout <= 0 {
		return libvirt_rpc.DomainQemuAgentCommandDefault
	}
	seconds := int32((timeout + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return seconds
}

// Command 执行一条结构化 QGA 命令并解析 return 字段。
func (c *Client) Command(ctx context.Context, name string, arguments any, output any, timeout time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	payload := map[string]any{"execute": name}
	if arguments != nil {
		payload["arguments"] = arguments
	}
	request, err := marshalAgentCommand(payload)
	if err != nil {
		return fmt.Errorf("构造 QEMU Guest Agent 命令失败: %w", err)
	}
	logger.App.Debug("执行 QEMU Guest Agent 命令", "vm", c.VMName, "command", name)
	raw, err := libvirt_rpc.QemuAgentCommandRPC(c.VMName, string(request), durationSeconds(timeout))
	if err != nil {
		return err
	}
	var envelope commandEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return fmt.Errorf("解析 QEMU Guest Agent 命令 %s 返回失败: %w", name, err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("QEMU Guest Agent 命令 %s 失败: %s", name, envelope.Error.Desc)
	}
	if output != nil && len(envelope.Return) > 0 {
		if err := json.Unmarshal(envelope.Return, output); err != nil {
			return fmt.Errorf("解析 QEMU Guest Agent 命令 %s 结果失败: %w", name, err)
		}
	}
	return nil
}

// marshalAgentCommand 将 QGA 请求编码为纯 ASCII JSON。
// 部分 QEMU Guest Agent 版本会拒绝 JSON 字符串中的原始 UTF-8 字节，
// 使用标准 JSON Unicode 转义可兼容中文脚本、路径和密码，同时不改变来宾收到的参数。
func marshalAgentCommand(payload any) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(encoded))
	for _, char := range string(encoded) {
		if char <= 0x7f {
			result = append(result, byte(char))
			continue
		}
		if char <= 0xffff {
			result = fmt.Appendf(result, `\u%04x`, char)
			continue
		}
		high, low := utf16.EncodeRune(char)
		result = fmt.Appendf(result, `\u%04x\u%04x`, high, low)
	}
	return result, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.Command(ctx, "guest-ping", nil, nil, ConnectTimeout)
}

func (c *Client) Info(ctx context.Context) (*AgentInfo, error) {
	var info AgentInfo
	if err := c.Command(ctx, "guest-info", nil, &info, ConnectTimeout); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) OSInfo(ctx context.Context) (*OSInfo, error) {
	var info OSInfo
	if err := c.Command(ctx, "guest-get-osinfo", nil, &info, ConnectTimeout); err != nil {
		return nil, err
	}
	return &info, nil
}

func (c *Client) Supports(ctx context.Context, command string) bool {
	info, err := c.Info(ctx)
	if err != nil {
		return false
	}
	for _, item := range info.SupportedCommands {
		if item.Name == command {
			return item.Enabled
		}
	}
	return false
}

func (c *Client) Disks(ctx context.Context) ([]GuestDisk, error) {
	var disks []GuestDisk
	if err := c.Command(ctx, "guest-get-disks", nil, &disks, ConnectTimeout); err != nil {
		return nil, err
	}
	return disks, nil
}

func (c *Client) Filesystems(ctx context.Context) ([]GuestFilesystem, error) {
	var filesystems []GuestFilesystem
	if err := c.Command(ctx, "guest-get-fsinfo", nil, &filesystems, ConnectTimeout); err != nil {
		return nil, err
	}
	return filesystems, nil
}

// Execute 在来宾中执行进程并等待完成。
func (c *Client) Execute(ctx context.Context, path string, args []string, timeout time.Duration) (*ExecResult, error) {
	if timeout <= 0 {
		timeout = ExecuteTimeout
	}
	var started struct {
		PID int `json:"pid"`
	}
	if err := c.Command(ctx, "guest-exec", map[string]any{
		"path": path, "arg": args, "capture-output": true,
	}, &started, ConnectTimeout); err != nil {
		return nil, err
	}
	if started.PID <= 0 {
		return nil, fmt.Errorf("QEMU Guest Agent 未返回有效进程号")
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("来宾命令执行超时")
		case <-ticker.C:
			var status struct {
				Exited   bool   `json:"exited"`
				ExitCode int    `json:"exitcode"`
				OutData  string `json:"out-data"`
				ErrData  string `json:"err-data"`
			}
			if err := c.Command(ctx, "guest-exec-status", map[string]any{"pid": started.PID}, &status, ConnectTimeout); err != nil {
				return nil, err
			}
			if !status.Exited {
				continue
			}
			result := &ExecResult{ExitCode: status.ExitCode}
			result.Stdout = decodeCapturedOutput(status.OutData)
			result.Stderr = decodeCapturedOutput(status.ErrData)
			return result, nil
		}
	}
}

func decodeCapturedOutput(value string) string {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return ""
	}
	if len(decoded) > maxOutputBytes {
		decoded = decoded[:maxOutputBytes]
	}
	return string(decoded)
}

// SetUserPassword 使用 QGA 在线设置来宾用户密码。
func (c *Client) SetUserPassword(ctx context.Context, username, password string) error {
	return c.Command(ctx, "guest-set-user-password", map[string]any{
		"username": strings.TrimSpace(username),
		"password": base64.StdEncoding.EncodeToString([]byte(password)),
		"crypted":  false,
	}, nil, ExecuteTimeout)
}
