/**
 * 挂载存储池到虚拟机弹窗：选择 VM + 存储类别 + 访问模式 → 9p VirtFS 挂载
 * 迁移自旧前端 views/storage/index.vue 的 mount dialog
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Modal, Radio, Select, Tag, Toast } from '@douyinfe/semi-ui'
import { getSelfVMs, type VmListItem } from '@/api/vm'
import { mountStorage } from '@/api/storage'
import { useUserStore } from '@/stores/user'
import TextSwitch from '@/features/vm-form/sections/TextSwitch'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface MountDialogProps {
  /** 默认选中的存储类别 */
  defaultCategory: string
  onClose: () => void
  /** 挂载成功后回调（显示挂载命令说明） */
  onMounted: (tag: string, readonly: boolean) => void
}

export default function MountDialog({ defaultCategory, onClose, onMounted }: MountDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const username = useUserStore((s) => s.username)
  const [vmList, setVmList] = useState<VmListItem[]>([])
  const [vmListLoading, setVmListLoading] = useState(false)
  const [vmName, setVmName] = useState('')
  const [category, setCategory] = useState(defaultCategory || 'share')
  const [readonly, setReadonly] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // ==================== 加载 VM 列表 ====================
  useEffect(() => {
    const load = async () => {
      setVmListLoading(true)
      try {
        const res = await getSelfVMs()
        setVmList(res.data || [])
      } catch (err) {
        console.error('获取虚拟机列表失败', err)
      } finally {
        setVmListLoading(false)
      }
    }
    void load()
  }, [])

  // ==================== 提交挂载 ====================
  const handleSubmit = async () => {
    if (!vmName) {
      Toast.warning('请选择虚拟机')
      return
    }
    setSubmitting(true)
    try {
      await mountStorage({ vm_name: vmName, category, readonly })
      const tag = `user_${username}_${category}`
      Toast.success('存储池已挂载到虚拟机')
      onMounted(tag, readonly)
      requestClose()
    } catch (err) {
      console.error('挂载失败', err)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      title="挂载存储池到虚拟机"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={500}
      footer={
        <>
          <Button onClick={requestClose}>取消</Button>
          <Button type="primary" loading={submitting} onClick={() => void handleSubmit()}>
            确认挂载
          </Button>
        </>
      }
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        {/* 虚拟机选择 */}
        <div>
          <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--qvm-text-1)', marginBottom: 6 }}>
            虚拟机
          </div>
          <Select
            value={vmName}
            onChange={(v) => setVmName(v as string)}
            placeholder="选择虚拟机"
            style={{ width: '100%' }}
            loading={vmListLoading}
            filter
          >
            {vmList.map((vm) => (
              <Select.Option key={vm.name} value={vm.name}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
                  <span>{vm.name}</span>
                  <Tag size="small" color={vm.status === 'running' ? 'green' : 'grey'}>
                    {vm.status === 'running' ? '运行中' : '已关机'}
                  </Tag>
                </div>
              </Select.Option>
            ))}
          </Select>
        </div>

        {/* 存储类别 */}
        <div>
          <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--qvm-text-1)', marginBottom: 6 }}>
            存储类别
          </div>
          <Radio.Group
            type="button"
            value={category}
            onChange={(e) => setCategory(e.target.value as string)}
            options={[
              { value: 'iso', label: 'ISO 镜像' },
              { value: 'share', label: '文件共享' },
            ]}
          />
        </div>

        {/* 访问模式 */}
        <div>
          <div style={{ fontSize: 13, fontWeight: 500, color: 'var(--qvm-text-1)', marginBottom: 6 }}>
            访问模式
          </div>
          <TextSwitch
            checked={readonly}
            onChange={setReadonly}
            checkedText="只"
            uncheckedText="写"
          />
        </div>

        {/* 提示 */}
        <Banner
          type="info"
          closeIcon={null}
          description="通过 9p VirtFS 协议共享目录到 Linux 虚拟机（Windows 不支持）"
        />
      </div>
    </Modal>
  )
}
