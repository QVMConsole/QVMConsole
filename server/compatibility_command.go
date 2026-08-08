package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"kvm_console/config"
	"kvm_console/logger"
	"kvm_console/model"
	"kvm_console/service/compatibility"
	"kvm_console/service/libvirt_rpc"
)

func runSystemCompatibilityCheckCommand(args []string) int {
	flags := flag.NewFlagSet("system-compatibility-check", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	vcpu := flags.Int("vcpu", 1, "测试虚拟机当前 vCPU 数量")
	ramGB := flags.Int("ram-gb", 1, "测试虚拟机内存大小（GB）")
	diskGB := flags.Int("disk-gb", 1, "测试虚拟机系统盘大小（GB）")
	reportDir := flags.String("report-dir", filepath.Join("logs", "compatibility"), "兼容性报告输出目录")
	flags.Usage = func() {
		_, _ = fmt.Fprintln(flags.Output(), "用法: kvm-console system-compatibility-check [选项]")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintf(os.Stderr, "解析兼容性测试参数失败: %v\n", err)
		return 2
	}
	if *vcpu <= 0 || *ramGB <= 0 || *diskGB <= 0 {
		_, _ = fmt.Fprintln(os.Stderr, "vCPU、内存和磁盘参数必须大于 0")
		return 2
	}
	if strings.TrimSpace(*reportDir) == "" {
		_, _ = fmt.Fprintln(os.Stderr, "兼容性报告目录不能为空")
		return 2
	}

	ensureLargeTempDir()
	config.Init()
	logger.InitWithConsoleConfig(
		config.GlobalConfig.LogDir,
		config.GlobalConfig.LogLevel,
		config.GlobalConfig.LogMaxDays,
		config.GlobalConfig.LogCompress,
		config.GlobalConfig.LogConsole,
		config.GlobalConfig.LogConsoleTypes,
		config.GlobalConfig.LogConsoleLevel,
		config.GlobalConfig.LogMaxSizeMB,
		config.GlobalConfig.LogMaxBackups,
	)
	defer logger.Close()

	model.InitDB()
	if savedSettings, err := model.GetAllSettings(); err == nil && len(savedSettings) > 0 {
		config.GlobalConfig.LoadFromDB(savedSettings)
	}
	libvirtInitErr := libvirt_rpc.InitLibvirtRPC()
	if libvirtInitErr == nil {
		defer libvirt_rpc.CloseLibvirt()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalChannel)
	go func() {
		<-signalChannel
		_, _ = fmt.Fprintln(os.Stderr, "收到中断信号，将在当前命令返回后清理测试虚拟机，请勿强制结束进程。")
		cancel()
	}()

	report, err := compatibility.RunSystemCheck(compatibility.Options{
		VCPU:      *vcpu,
		RAMGB:     *ramGB,
		DiskGB:    *diskGB,
		ReportDir: *reportDir,
		Context:   ctx,
		InitError: libvirtInitErr,
	}, func(message string) {
		_, _ = fmt.Fprintf(os.Stdout, "[兼容性测试] %s\n", message)
	})
	if report != nil {
		_, _ = fmt.Fprintf(os.Stdout, "兼容性报告: %s\n", report.ReportPath)
		if report.XMLPath != "" {
			_, _ = fmt.Fprintf(os.Stdout, "持久化测试 XML: %s\n", report.XMLPath)
		}
		if report.ActiveXMLPath != "" {
			_, _ = fmt.Fprintf(os.Stdout, "运行态测试 XML: %s\n", report.ActiveXMLPath)
		}
		if report.DiagnosticLog != "" {
			_, _ = fmt.Fprintf(os.Stdout, "诊断日志: %s\n", report.DiagnosticLog)
		}
		for index := len(report.Stages) - 1; index >= 0; index-- {
			if report.Stages[index].Status == "failed" {
				_, _ = fmt.Fprintf(os.Stderr, "失败阶段: %s（%s）\n", report.Stages[index].Name, report.Stages[index].Message)
				break
			}
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "系统兼容性测试未通过: %v\n", err)
		if errors.Is(err, context.Canceled) {
			return 130
		}
		return 1
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		_, _ = fmt.Fprintln(os.Stderr, "系统兼容性测试已中断，临时资源已清理。")
		return 130
	}
	_, _ = io.WriteString(os.Stdout, "系统兼容性测试通过：虚拟机已成功创建、接入基础 OVS 网络并启动。\n")
	return 0
}
