package portmirror

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type legacyConfig struct {
	Enabled          bool      `json:"enabled"`
	SourceInterface  string    `json:"source_interface"`
	TargetSwitchID   uint      `json:"target_switch_id"`
	TargetSwitchName string    `json:"target_switch_name"`
	TargetBridge     string    `json:"target_bridge"`
	Direction        string    `json:"direction"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type legacyRuntimeState struct {
	Enabled          bool      `json:"enabled"`
	SourceInterface  string    `json:"source_interface"`
	TargetSwitchID   uint      `json:"target_switch_id"`
	TargetSwitchName string    `json:"target_switch_name"`
	TargetBridge     string    `json:"target_bridge"`
	Direction        string    `json:"direction"`
	UpdatedAt        time.Time `json:"updated_at"`
	VethSource       string    `json:"veth_source"`
	OVSPort          string    `json:"ovs_port"`
	OFPort           int       `json:"ofport"`
	Cookie           string    `json:"cookie"`
	ClsactCreated    bool      `json:"clsact_created"`
}

func migrateLegacyConfig(legacy legacyConfig) *Config {
	if !legacy.Enabled || legacy.SourceInterface == "" || legacy.TargetSwitchID == 0 || legacy.TargetBridge == "" {
		return nil
	}
	return &Config{
		Enabled:          true,
		SourceInterfaces: []string{legacy.SourceInterface},
		Targets: []TargetConfig{{
			SwitchID: legacy.TargetSwitchID, SwitchName: legacy.TargetSwitchName, Bridge: legacy.TargetBridge,
		}},
		Direction: legacy.Direction,
		UpdatedAt: legacy.UpdatedAt,
	}
}

func migrateLegacyRuntime(legacy legacyRuntimeState) *RuntimeState {
	cfg := migrateLegacyConfig(legacyConfig{
		Enabled: legacy.Enabled, SourceInterface: legacy.SourceInterface,
		TargetSwitchID: legacy.TargetSwitchID, TargetSwitchName: legacy.TargetSwitchName,
		TargetBridge: legacy.TargetBridge, Direction: legacy.Direction, UpdatedAt: legacy.UpdatedAt,
	})
	if cfg == nil || legacy.VethSource == "" || legacy.OVSPort == "" || legacy.Cookie == "" {
		return nil
	}
	return &RuntimeState{
		Config:  *cfg,
		Sources: []RuntimeSource{{Name: legacy.SourceInterface, ClsactCreated: legacy.ClsactCreated}},
		Connections: []RuntimeConnection{{
			SourceInterface: legacy.SourceInterface,
			TargetSwitchID:  legacy.TargetSwitchID,
			TargetBridge:    legacy.TargetBridge,
			VethSource:      legacy.VethSource,
			OVSPort:         legacy.OVSPort,
			OFPort:          legacy.OFPort,
			Cookie:          legacy.Cookie,
		}},
	}
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("解析端口镜像状态文件失败: %w", err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化端口镜像配置失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建端口镜像配置目录失败: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".port-mirror-*.tmp")
	if err != nil {
		return fmt.Errorf("创建端口镜像临时配置失败: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("提交端口镜像配置失败: %w", err)
	}
	return nil
}

func loadConfig() (*Config, error) {
	var cfg Config
	if err := readJSON(ConfigPath, &cfg); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if !cfg.Enabled || (len(cfg.SourceInterfaces) > 0 && len(cfg.Targets) > 0) {
		return &cfg, nil
	}
	var legacy legacyConfig
	if err := readJSON(ConfigPath, &legacy); err != nil {
		return nil, err
	}
	if migrated := migrateLegacyConfig(legacy); migrated != nil {
		return migrated, nil
	}
	return nil, fmt.Errorf("端口镜像配置缺少来源或目标")
}

func loadRuntime() (*RuntimeState, error) {
	var state RuntimeState
	if err := readJSON(RuntimePath, &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(state.Sources) > 0 && len(state.Connections) > 0 {
		return &state, nil
	}
	var legacy legacyRuntimeState
	if err := readJSON(RuntimePath, &legacy); err != nil {
		return nil, err
	}
	if migrated := migrateLegacyRuntime(legacy); migrated != nil {
		return migrated, nil
	}
	return &state, nil
}
