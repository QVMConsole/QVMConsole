package host

import (
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"kvm_console/config"
	"kvm_console/model"
)

const (
	HostNodeStatusUnknown = "unknown"
	HostNodeStatusOnline  = "online"
	HostNodeStatusError   = "error"
	requiredNodeSSHUser   = "root"
)

func ListHostNodes() ([]HostNodeView, error) {
	var nodes []model.HostNode
	if err := model.DB.Order("id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	result := make([]HostNodeView, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, BuildHostNodeView(node))
	}
	return result, nil
}

func GetHostNode(id uint) (*model.HostNode, error) {
	var node model.HostNode
	if err := model.DB.First(&node, id).Error; err != nil {
		return nil, fmt.Errorf("节点不存在")
	}
	return &node, nil
}

func CreateHostNode(req HostNodeRequest) (*HostNodeView, error) {
	node, err := buildHostNodeFromRequest(model.HostNode{}, req, true)
	if err != nil {
		return nil, err
	}
	if err := model.DB.Create(&node).Error; err != nil {
		return nil, fmt.Errorf("保存节点失败: %w", err)
	}
	view := BuildHostNodeView(node)
	return &view, nil
}

func UpdateHostNode(id uint, req HostNodeRequest) (*HostNodeView, error) {
	var current model.HostNode
	if err := model.DB.First(&current, id).Error; err != nil {
		return nil, fmt.Errorf("节点不存在")
	}
	node, err := buildHostNodeFromRequest(current, req, false)
	if err != nil {
		return nil, err
	}
	if err := model.DB.Save(&node).Error; err != nil {
		return nil, fmt.Errorf("更新节点失败: %w", err)
	}
	view := BuildHostNodeView(node)
	return &view, nil
}

func DeleteHostNode(id uint) error {
	if err := model.DB.Delete(&model.HostNode{}, id).Error; err != nil {
		return fmt.Errorf("删除节点失败: %w", err)
	}
	return nil
}

func ProbeHostNode(id uint) (*HostNodeView, error) {
	node, err := GetHostNode(id)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(node.SSHUser) != requiredNodeSSHUser {
		message := "节点 SSH 用户必须为 root，非 root 用户没有迁移虚拟机所需的 libvirt、OVS 与存储目录权限"
		updateHostNodeProbe(node, HostNodeStatusError, message, map[string]interface{}{
			"ssh_user": "必须为 root",
		})
		view := BuildHostNodeView(*node)
		return &view, fmt.Errorf("%s", message)
	}
	caps := map[string]interface{}{}
	checks := []string{
		"command -v virsh",
		"command -v qemu-img",
		"command -v rsync",
		"command -v ssh",
		"virsh --version",
		"qemu-img --version | head -1",
		"test -d /var/lib/libvirt/images && echo images-ok",
		"test -d /var/lib/libvirt/images/templates && echo templates-ok",
		"ovs-vsctl br-exists br-ovs && echo br-ovs-ok",
	}
	var firstSSHErr string
	for i, check := range checks {
		out, err := HookRemoteSSHExec(nil, *node, check, 30*time.Second, true)
		key := fmt.Sprintf("check_%02d", i+1)
		if err != nil {
			updateHostNodeProbe(node, HostNodeStatusError, "节点探测失败: "+err.Error(), caps)
			view := BuildHostNodeView(*node)
			return &view, err
		}
		trimmed := strings.TrimSpace(out)
		if trimmed == "" {
			if firstSSHErr == "" {
				firstSSHErr = fmt.Sprintf("检查 %s 未通过（命令未返回期望结果）", check)
			}
			caps[key] = "未通过"
		} else {
			caps[key] = trimmed
		}
	}
	if _, err := HookCallNodeAPI(*node, "GET", "/api/public/settings", nil, nil); err != nil {
		updateHostNodeProbe(node, HostNodeStatusError, "面板 API 探测失败: "+err.Error(), caps)
		view := BuildHostNodeView(*node)
		return &view, err
	}
	if firstSSHErr != "" {
		updateHostNodeProbe(node, HostNodeStatusError, "部分检查未通过: "+firstSSHErr, caps)
		view := BuildHostNodeView(*node)
		return &view, fmt.Errorf("部分检查未通过: %s", firstSSHErr)
	}
	updateHostNodeProbe(node, HostNodeStatusOnline, "节点探测通过", caps)
	view := BuildHostNodeView(*node)
	return &view, nil
}

func buildHostNodeFromRequest(current model.HostNode, req HostNodeRequest, creating bool) (model.HostNode, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.APIBaseURL = normalizeNodeBaseURL(req.APIBaseURL)
	req.APIKeyID = strings.TrimSpace(req.APIKeyID)
	req.SSHHost = strings.TrimSpace(req.SSHHost)
	req.SSHUser = strings.TrimSpace(req.SSHUser)
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = requiredNodeSSHUser
	}
	if req.SSHUser != requiredNodeSSHUser {
		return current, fmt.Errorf("节点 SSH 用户必须为 root，非 root 用户没有迁移虚拟机所需的 libvirt、OVS 与存储目录权限")
	}
	if !creating && strings.TrimSpace(current.SSHUser) != requiredNodeSSHUser && strings.TrimSpace(req.SSHPassword) == "" {
		return current, fmt.Errorf("该节点原 SSH 用户不是 root，请输入目标节点 root 密码后保存")
	}
	if req.Name == "" || req.APIBaseURL == "" || req.SSHHost == "" {
		return current, fmt.Errorf("节点名称、面板地址和 SSH 地址不能为空")
	}
	if _, err := url.ParseRequestURI(req.APIBaseURL); err != nil {
		return current, fmt.Errorf("面板 API 地址格式无效")
	}
	current.Name = req.Name
	current.APIBaseURL = strings.TrimRight(req.APIBaseURL, "/")
	current.APIKeyID = req.APIKeyID
	current.SSHHost = req.SSHHost
	current.SSHPort = req.SSHPort
	current.SSHUser = req.SSHUser
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	} else if creating {
		current.Enabled = true
	}
	if creating {
		current.Status = HostNodeStatusUnknown
	}
	if strings.TrimSpace(req.APIKey) != "" {
		enc, err := EncryptNodeSecret(req.APIKey)
		if err != nil {
			return current, fmt.Errorf("加密 API Key 失败: %w", err)
		}
		current.APIKeyEnc = enc
	}
	if creating && strings.TrimSpace(current.APIKeyEnc) == "" {
		return current, fmt.Errorf("目标面板 API Key 不能为空")
	}
	if strings.TrimSpace(req.SSHPassword) != "" {
		enc, err := EncryptNodeSecret(req.SSHPassword)
		if err != nil {
			return current, fmt.Errorf("加密 root 密码失败: %w", err)
		}
		current.SSHPasswordEnc = enc
	}
	if creating && strings.TrimSpace(current.SSHPasswordEnc) == "" {
		return current, fmt.Errorf("SSH root 密码不能为空")
	}
	return current, nil
}

func BuildHostNodeView(node model.HostNode) HostNodeView {
	caps := map[string]interface{}{}
	if strings.TrimSpace(node.CapabilitiesJSON) != "" {
		_ = json.Unmarshal([]byte(node.CapabilitiesJSON), &caps)
	}
	return HostNodeView{
		ID:               node.ID,
		Name:             node.Name,
		APIBaseURL:       node.APIBaseURL,
		APIKeyID:         node.APIKeyID,
		SSHHost:          node.SSHHost,
		SSHPort:          node.SSHPort,
		SSHUser:          node.SSHUser,
		Enabled:          node.Enabled,
		Status:           node.Status,
		LastProbeMessage: node.LastProbeMessage,
		Capabilities:     caps,
		LastProbedAt:     node.LastProbedAt,
		CreatedAt:        node.CreatedAt,
		UpdatedAt:        node.UpdatedAt,
	}
}

func updateHostNodeProbe(node *model.HostNode, status, message string, caps map[string]interface{}) {
	now := time.Now()
	payload, _ := json.Marshal(caps)
	node.Status = status
	node.LastProbeMessage = message
	node.CapabilitiesJSON = string(payload)
	node.LastProbedAt = &now
	_ = model.DB.Save(node).Error
}

func normalizeNodeBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimRight(value, "/")
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "http://" + value
}

func DecryptHostNodeSSHPassword(node model.HostNode) (string, error) {
	return DecryptNodeSecret(node.SSHPasswordEnc)
}

func DecryptHostNodeAPIKey(node model.HostNode) (string, error) {
	return DecryptNodeSecret(node.APIKeyEnc)
}

func EncryptNodeSecret(plainText string) (string, error) {
	block, err := aes.NewCipher(BuildNodeSecretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(crand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(nil, nonce, []byte(plainText), nil)
	payload := append(nonce, cipherText...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func DecryptNodeSecret(cipherText string) (string, error) {
	if strings.TrimSpace(cipherText) == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(BuildNodeSecretKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("密文格式无效")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func BuildNodeSecretKey() []byte {
	seed := config.GlobalConfig.VMCredentialSecret
	if strings.TrimSpace(seed) == "" {
		seed = config.GlobalConfig.JWTSecret
	}
	sum := sha256.Sum256([]byte("node:" + seed))
	return sum[:]
}
