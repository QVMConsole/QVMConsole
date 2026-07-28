/**
 * 恢复码展示弹窗
 * - 启用 2FA / 重新生成恢复码后仅展示一次
 * - 编号网格展示，支持一键复制与下载为文本文件
 */
import { useState } from 'react'
import { Button, Modal, Toast } from '@douyinfe/semi-ui'
import { IconCopy, IconDownload } from '@douyinfe/semi-icons'
import { copyTextWithFallback } from '@/utils/clipboard'
import './recovery-codes.css'

interface RecoveryCodesModalProps {
  visible: boolean
  codes: string[]
  /** 关闭弹窗（父组件需同时清空恢复码缓存） */
  onClose: () => void
}

export default function RecoveryCodesModal({ visible, codes, onClose }: RecoveryCodesModalProps) {
  const [copying, setCopying] = useState(false)

  const handleCopy = async () => {
    setCopying(true)
    try {
      await copyTextWithFallback(codes.join('\n'))
      Toast.success('恢复码已复制')
    } catch (err) {
      Toast.error(err instanceof Error ? err.message : '复制失败')
    } finally {
      setCopying(false)
    }
  }

  const handleDownload = () => {
    const blob = new Blob([codes.join('\n')], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'qvmconsole-recovery-codes.txt'
    a.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Modal
      title="请保存恢复码"
      visible={visible}
      onCancel={onClose}
      closable={false}
      maskClosable={false}
      footer={
        <div className="sec-recovery-footer">
          <Button icon={<IconCopy />} loading={copying} onClick={() => void handleCopy()}>
            复制全部
          </Button>
          <Button icon={<IconDownload />} onClick={handleDownload}>
            下载
          </Button>
          <Button type="primary" theme="solid" onClick={onClose}>
            我已安全保存
          </Button>
        </div>
      }
    >
      <div className="sec-recovery-warn">以下恢复码仅在本次显示，请立即复制或下载保存：</div>
      <div className="sec-recovery-grid">
        {codes.map((code, index) => (
          <div key={code} className="sec-recovery-item">
            <span className="sec-recovery-no">{String(index + 1).padStart(2, '0')}.</span>
            <span className="sec-recovery-code">{code}</span>
          </div>
        ))}
      </div>
      <div className="sec-recovery-tip">
        当 2FA 验证器不可用时，可使用恢复码登录。每个恢复码只能使用一次。
      </div>
    </Modal>
  )
}
