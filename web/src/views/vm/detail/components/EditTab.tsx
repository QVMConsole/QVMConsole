/**
 * 编辑 Tab（占位）
 * 后续版本将复用「创建虚拟机表单」实现虚拟机硬件配置编辑，本轮仅提供占位说明。
 */
import { Empty } from '@douyinfe/semi-ui'
import { IllustrationConstruction, IllustrationConstructionDark } from '@douyinfe/semi-illustrations'

export default function EditTab() {
  return (
    <div className="qvm-edit-tab">
      <Empty
        image={<IllustrationConstruction />}
        darkModeImage={<IllustrationConstructionDark />}
        title="编辑功能建设中"
        description="虚拟机硬件配置编辑将在后续版本提供，届时会复用创建虚拟机的表单实现统一体验。"
      />
    </div>
  )
}
