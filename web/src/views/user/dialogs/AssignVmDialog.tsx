/**
 * 分配虚拟机弹窗（弹性云用户）
 * 全量覆盖式分配：提交后以所选列表为准更新用户名下 VM
 */
import { useEffect, useState } from 'react'
import { Banner, Modal, Select, Toast } from '@douyinfe/semi-ui'
import { assignVms, type UserListItem } from '@/api/user'
import { getVmList } from '@/api/vm'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface AssignVmDialogProps {
  row: UserListItem
  onClose: () => void
  onSaved: () => void
}

export default function AssignVmDialog({ row, onClose, onSaved }: AssignVmDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [submitting, setSubmitting] = useState(false)
  const [loadingVms, setLoadingVms] = useState(false)
  const [allVms, setAllVms] = useState<string[]>([])
  const [selected, setSelected] = useState<string[]>(row.vms ? [...row.vms] : [])

  // 加载全部虚拟机名称
  useEffect(() => {
    let mounted = true
    const load = async () => {
      setLoadingVms(true)
      try {
        const res = await getVmList()
        if (mounted) setAllVms((res.data || []).map((vm) => vm.name))
      } catch {
        // 请求层已提示
      } finally {
        if (mounted) setLoadingVms(false)
      }
    }
    void load()
    return () => {
      mounted = false
    }
  }, [])

  const handleSubmit = async () => {
    setSubmitting(true)
    try {
      await assignVms(row.username, { vms: selected })
      Toast.success('分配成功')
      onSaved()
      requestClose()
    } catch {
      // 请求层已提示
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="分配虚拟机"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      onOk={() => void handleSubmit()}
      okText="保存"
      cancelText="取消"
      confirmLoading={submitting}
      width={640}
      closeOnEsc
    >
      <Banner
        type="info"
        closeIcon={null}
        description={
          <span>
            用户：<strong>{row.username}</strong>，保存后以所选列表为准更新分配关系。
          </span>
        }
        style={{ marginBottom: 16 }}
      />
      <div className="qvm-form-item">
        <div className="qvm-form-label">虚拟机</div>
        <Select
          multiple
          filter
          value={selected}
          onChange={(v) => setSelected(v as string[])}
          loading={loadingVms}
          placeholder="选择虚拟机"
          style={{ width: '100%' }}
          maxTagCount={8}
          optionList={allVms.map((name) => ({ label: name, value: name }))}
        />
      </div>
    </Modal>
  )
}
