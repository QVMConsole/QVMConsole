/**
 * 虚拟机 XML 编辑弹窗（编辑模式）
 * 查看 / 编辑持久化 domain XML，保存后立即写入 libvirt 定义。
 */
import { useCallback, useEffect, useState } from 'react'
import { Banner, Button, Modal, Tag, TextArea, Toast } from '@douyinfe/semi-ui'
import { getVmXML, updateVmXML } from '@/api/vm'

interface VmXmlDialogProps {
  visible: boolean
  vmName: string
  vmStatus: string
  onClose: () => void
  /** 保存成功后回调（用于刷新表单与磁盘列表） */
  onSaved: () => void | Promise<void>
}

const normalizeXml = (value: string) => `${value || ''}`.replace(/\r\n/g, '\n').trim()

export default function VmXmlDialog({ visible, vmName, vmStatus, onClose, onSaved }: VmXmlDialogProps) {
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [content, setContent] = useState('')
  const [original, setOriginal] = useState('')

  const loadXml = useCallback(
    async (showSuccess = false) => {
      if (!vmName) return
      setLoading(true)
      try {
        const res = await getVmXML(vmName)
        const xml = res.data?.xml || ''
        setContent(xml)
        setOriginal(xml)
        if (showSuccess) Toast.success('XML 已重新加载')
      } catch {
        // 错误由请求层统一提示
      } finally {
        setLoading(false)
      }
    },
    [vmName],
  )

  useEffect(() => {
    if (visible) void loadXml()
  }, [visible, loadXml])

  const dirty = normalizeXml(content) !== normalizeXml(original)
  const runningOrPaused = vmStatus === 'running' || vmStatus === 'paused'

  const handleSave = async () => {
    if (!vmName || !normalizeXml(content) || !dirty) return
    const confirmed = await new Promise<boolean>((resolve) => {
      Modal.confirm({
        title: '保存虚拟机 XML',
        content: '保存会立即写入当前虚拟机的持久化 XML，并刷新当前表单。建议先关机后再执行保存，是否继续？',
        okText: '继续保存',
        cancelText: '取消',
        onOk: () => resolve(true),
        onCancel: () => resolve(false),
      })
    })
    if (!confirmed) return
    setSaving(true)
    try {
      await updateVmXML(vmName, { xml: content })
      Toast.success('虚拟机 XML 保存成功')
      await loadXml()
      await onSaved()
    } catch {
      // 错误由请求层统一提示
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title="编辑虚拟机 XML"
      visible={visible}
      onCancel={onClose}
      width={920}
      closeOnEsc
      footer={
        <>
          <Button loading={loading} disabled={saving} onClick={() => void loadXml(true)}>
            重新加载
          </Button>
          <Button onClick={onClose}>关闭</Button>
          <Button
            type="primary"
            theme="solid"
            loading={saving}
            disabled={loading || !dirty || !normalizeXml(content)}
            onClick={() => void handleSave()}
          >
            保存 XML
          </Button>
        </>
      }
    >
      <Banner
        type="warning"
        closeIcon={null}
        style={{ marginBottom: 12 }}
        description="这里编辑的是当前虚拟机的持久化 domain XML。保存后会立即写入 libvirt 定义，并刷新当前表单；未提交的普通设置改动会被覆盖，且不支持通过此功能修改虚拟机名称。"
      />
      <div className="qvm-vf-xml-toolbar">
        <Tag color={runningOrPaused ? 'orange' : 'green'} size="small">
          {runningOrPaused ? '运行中：保存后通常需重启生效' : '已关机：可直接保存持久化配置'}
        </Tag>
        <span className="qvm-vf-tip">建议先关机后再修改，以免运行态与持久化配置出现理解偏差。</span>
      </div>
      <TextArea
        value={content}
        onChange={setContent}
        rows={22}
        disabled={loading || saving}
        placeholder="正在加载虚拟机 XML..."
        style={{ fontFamily: 'monospace', fontSize: 12.5 }}
      />
    </Modal>
  )
}
