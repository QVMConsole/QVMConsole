/**
 * 创建方式选择分区（向导第 1 步）
 * ISO 镜像安装 / 模板快速克隆 / 导入已有磁盘 / 导入虚拟机四卡片选择。
 */
import { IconBox, IconDisc, IconCopy, IconImport } from '@douyinfe/semi-icons'
import { useVmFormScope } from '../scopeContext'
import type { VmCreateMode } from '../types'

const MODE_CARDS: { mode: VmCreateMode; icon: React.ReactNode; title: string; desc: string }[] = [
  {
    mode: 'iso',
    icon: <IconDisc size="extra-large" />,
    title: '使用 ISO 镜像安装',
    desc: '使用标准系统镜像安装全新操作环境，适合全新部署',
  },
  {
    mode: 'template',
    icon: <IconCopy size="extra-large" />,
    title: '从模板快速克隆',
    desc: '基于预配置模板秒级创建，适合批量部署',
  },
  {
    mode: 'import',
    icon: <IconBox size="extra-large" />,
    title: '导入已有磁盘',
    desc: '使用已有的虚拟机磁盘文件快速运行',
  },
  {
    mode: 'appliance',
    icon: <IconImport size="extra-large" />,
    title: '导入虚拟机',
    desc: '从 OVF 或 OVA 虚拟机包恢复配置和磁盘',
  },
]

interface CreateModeSectionProps {
  /** 选择方式后的回调（向导进入下一步） */
  onSelect: (mode: VmCreateMode) => void
}

export default function CreateModeSection({ onSelect }: CreateModeSectionProps) {
  const { form } = useVmFormScope()

  return (
    <div className="qvm-vf-mode-cards">
      {MODE_CARDS.map((card) => (
        <div
          key={card.mode}
          className={`qvm-vf-mode-card${form.form.create_mode === card.mode ? ' selected' : ''}`}
          onClick={() => onSelect(card.mode)}
        >
          <div className="qvm-vf-mode-card-icon">{card.icon}</div>
          <div className="qvm-vf-mode-card-title">{card.title}</div>
          <div className="qvm-vf-mode-card-desc">{card.desc}</div>
        </div>
      ))}
    </div>
  )
}
