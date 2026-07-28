/**
 * 关于项目页
 * 技术栈展示、项目信息、面板信息（/public/version）、系统运行环境信息（/system-info）。
 */
import { useEffect, useState } from 'react'
import { Card, Collapse, Spin, Tag } from '@douyinfe/semi-ui'
import { IconDesktop, IconLink, IconMonitorStroked, IconSetting } from '@douyinfe/semi-icons'
import {
  getPublicSystemInfo,
  getPublicVersion,
  type PublicSystemInfo,
  type PublicVersion,
} from '@/api/settings'
import './about.css'

/** 技术栈清单（新前端为 React + Semi Design 体系） */
const techStack = [
  { name: 'React 19', desc: '用于构建界面的 JavaScript 库', url: 'https://react.dev' },
  { name: 'Semi Design', desc: '现代化企业级 UI 组件库', url: 'https://semi.design' },
  { name: 'Vite', desc: '下一代前端构建工具', url: 'https://vitejs.dev' },
  { name: 'Zustand', desc: 'React 轻量状态管理库', url: 'https://zustand-demo.pmnd.rs' },
  { name: 'Go', desc: '高性能后端语言', url: 'https://go.dev' },
  { name: 'Gin', desc: 'Go HTTP Web 框架', url: 'https://gin-gonic.com' },
  { name: 'SQLite', desc: '轻量级嵌入式数据库', url: 'https://www.sqlite.org' },
  { name: 'libvirt', desc: '虚拟化管理 API', url: 'https://libvirt.org' },
  { name: 'QEMU/KVM', desc: '硬件虚拟化方案', url: 'https://www.qemu.org' },
  { name: 'noVNC', desc: 'Web 远程桌面客户端', url: 'https://novnc.com' },
]

/** 项目信息条目 */
const projectLinks = [
  { label: '开源地址', text: 'https://github.com/QVMConsole/QVMConsole', url: 'https://github.com/QVMConsole/QVMConsole' },
  { label: '项目官网', text: 'https://www.qvmconsole.cn/', url: 'https://www.qvmconsole.cn/' },
  { label: '项目文档', text: 'https://qvmcdocs.xiaozhuhouses.asia', url: 'https://qvmcdocs.xiaozhuhouses.asia' },
]

/** 系统信息展示字段映射 */
const sysInfoFields: { label: string; keys: string[] }[] = [
  { label: '操作系统', keys: ['distro', 'os'] },
  { label: '内核版本', keys: ['kernel'] },
  { label: '系统架构', keys: ['arch'] },
  { label: '主机名', keys: ['hostname'] },
  { label: 'CPU 核数', keys: ['num_cpu'] },
  { label: 'Go 版本', keys: ['go_version'] },
  { label: 'QEMU 版本', keys: ['qemu'] },
  { label: 'libvirt 版本', keys: ['libvirt'] },
  { label: '系统运行时间', keys: ['uptime'] },
]

export default function AboutPage() {
  const isDev = import.meta.env.DEV
  const currentYear = new Date().getFullYear()
  const [versionInfo, setVersionInfo] = useState<PublicVersion>({})
  const [sysInfo, setSysInfo] = useState<PublicSystemInfo>({})
  const [sysLoading, setSysLoading] = useState(false)

  useEffect(() => {
    let mounted = true
    const load = async () => {
      setSysLoading(true)
      try {
        const [verRes, sysRes] = await Promise.allSettled([getPublicVersion(), getPublicSystemInfo()])
        if (!mounted) return
        if (verRes.status === 'fulfilled') setVersionInfo(verRes.value.data || {})
        else setVersionInfo({ version: 'dev' })
        if (sysRes.status === 'fulfilled') setSysInfo(sysRes.value.data || {})
      } finally {
        if (mounted) setSysLoading(false)
      }
    }
    void load()
    return () => {
      mounted = false
    }
  }, [])

  const sysValue = (keys: string[]): string => {
    for (const k of keys) {
      const v = sysInfo[k]
      if (v !== undefined && v !== null && v !== '') return String(v)
    }
    return '-'
  }

  return (
    <div className="about-page">
      <Collapse defaultActiveKey={['tech', 'project', 'panel', 'system']} keepDOM>
        <Collapse.Panel
          itemKey="tech"
          header={
            <span className="about-section-header">
              <IconDesktop className="about-section-icon" />
              技术栈
            </span>
          }
        >
          <div className="about-tech-grid">
            {techStack.map((tech) => (
              <a key={tech.name} className="about-tech-item" href={tech.url} target="_blank" rel="noopener noreferrer">
                <span className="about-tech-name">{tech.name}</span>
                <span className="about-tech-desc">{tech.desc}</span>
              </a>
            ))}
          </div>
        </Collapse.Panel>

        <Collapse.Panel
          itemKey="project"
          header={
            <span className="about-section-header">
              <IconLink className="about-section-icon" />
              项目信息
            </span>
          }
        >
          <div className="about-info-grid">
            {projectLinks.map((item) => (
              <div key={item.label} className="about-info-item">
                <span className="about-info-label">{item.label}</span>
                <a className="about-info-link" href={item.url} target="_blank" rel="noopener noreferrer">
                  {item.text}
                </a>
              </div>
            ))}
            <div className="about-info-item">
              <span className="about-info-label">开发者</span>
              <span className="about-info-value">
                星辰项目组-又菜又爱玩的小朱
                <a className="about-info-link" href="https://github.com/yxsj245" target="_blank" rel="noopener noreferrer">
                  (@yxsj245)
                </a>
              </span>
            </div>
          </div>
        </Collapse.Panel>

        <Collapse.Panel
          itemKey="panel"
          header={
            <span className="about-section-header">
              <IconMonitorStroked className="about-section-icon" />
              面板信息
            </span>
          }
        >
          <div className="about-info-grid">
            <div className="about-info-item">
              <span className="about-info-label">版本</span>
              <span className="about-info-value">{versionInfo.version || '开发版'}</span>
            </div>
            <div className="about-info-item">
              <span className="about-info-label">构建时间</span>
              <span className="about-info-value">{versionInfo.build_time || '未设置'}</span>
            </div>
            <div className="about-info-item">
              <span className="about-info-label">站点名称</span>
              <span className="about-info-value">{versionInfo.site_title || '未设置'}</span>
            </div>
            <div className="about-info-item">
              <span className="about-info-label">运行模式</span>
              <Tag size="small" color={isDev ? 'orange' : 'green'}>
                {isDev ? '开发环境' : '生产环境'}
              </Tag>
            </div>
          </div>
        </Collapse.Panel>

        <Collapse.Panel
          itemKey="system"
          header={
            <span className="about-section-header">
              <IconSetting className="about-section-icon" />
              系统信息
            </span>
          }
        >
          <Spin spinning={sysLoading}>
            <div className="about-info-grid">
              {sysInfoFields.map((f) => (
                <div key={f.label} className="about-info-item">
                  <span className="about-info-label">{f.label}</span>
                  <span className="about-info-value">{sysValue(f.keys)}</span>
                </div>
              ))}
            </div>
          </Spin>
        </Collapse.Panel>
      </Collapse>

      <Card className="about-footer-card">
        <p className="about-footer">© {currentYear} QVMConsole. 基于 React + Semi Design + Go 构建</p>
      </Card>
    </div>
  )
}
