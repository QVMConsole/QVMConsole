/**
 * 删除模板链路弹窗：级联删除 / 仅删除当前节点并提升子节点 / 热删除
 * 迁移自旧前端 views/template/index.vue 的删除对话框
 * 注：删除模板为高风险操作，428 二次验证由请求层自动处理
 */
import { useEffect, useMemo, useState } from 'react'
import { Banner, Button, Descriptions, Modal, Radio, Spin, Table, Toast } from '@douyinfe/semi-ui'
import {
  deleteTemplate,
  getTemplateDeletePreview,
  type DeleteTemplatePreview,
  type TemplateDeleteMode,
  type TemplateItem,
  type TemplateRelatedVM,
} from '@/api/template'
import { useMountModalLifecycle } from '@/hooks/useMountModalLifecycle'

interface DeleteTemplateChainDialogProps {
  node: TemplateItem
  onClose: () => void
  /** 删除任务提交成功后回调（用于刷新列表） */
  onDeleted: () => void
}

/** 三种删除模式的提示文案 */
const MODE_TIPS: Record<TemplateDeleteMode, string> = {
  cascade: '级联删除会移除当前节点及其所有子节点，并按确认范围删除关联虚拟机。',
  promote_children: '仅删除当前节点会安全 rebase 直接子模板和直接 VM，所有关联 VM 必须先关机。',
  promote_children_hot: '热删除会在线切换运行中 VM 的 backing，尽量不关机；失败时不会删除当前模板节点。',
}

export default function DeleteTemplateChainDialog({
  node,
  onClose,
  onDeleted,
}: DeleteTemplateChainDialogProps) {
  const { modalVisible, requestClose, afterModalClose } = useMountModalLifecycle(onClose)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [mode, setMode] = useState<TemplateDeleteMode>('cascade')
  const [preview, setPreview] = useState<DeleteTemplatePreview | null>(null)

  useEffect(() => {
    let cancelled = false
    ;(async () => {
      setLoading(true)
      try {
        const res = await getTemplateDeletePreview(node.name)
        if (!cancelled) setPreview(res.data || null)
      } catch (err) {
        console.error('获取模板删除预览失败', err)
        if (!cancelled) requestClose()
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node.name])

  const templates = useMemo(() => preview?.templates || [], [preview])
  const relatedVMs = useMemo(() => preview?.related_vms || [], [preview])
  const promotedTemplates = useMemo(() => preview?.promoted_templates || [], [preview])
  const rebasedVMs = useMemo(() => preview?.rebased_vms || [], [preview])
  const parentTemplate = preview?.parent_template || null

  const isPromoteMode = mode === 'promote_children' || mode === 'promote_children_hot'
  const blockers =
    mode === 'promote_children'
      ? preview?.promote_blockers || []
      : mode === 'promote_children_hot'
        ? preview?.promote_hot_blockers || []
        : []
  const confirmDisabled =
    (mode === 'promote_children' && !preview?.can_promote) ||
    (mode === 'promote_children_hot' && !preview?.can_promote_hot)

  const isDirectRebasedVM = (name: string) => rebasedVMs.some((vm) => vm.name === name)

  /** 关联 VM 的处理方式说明（提升模式） */
  const vmActionText = (vm: TemplateRelatedVM): string => {
    if (isDirectRebasedVM(vm.name)) {
      return mode === 'promote_children_hot' && vm.status === 'running'
        ? '在线拉平到上级'
        : 'rebase 到上级'
    }
    if (mode === 'promote_children_hot' && vm.status === 'running') {
      return '在线切换新 backing'
    }
    return mode === 'promote_children_hot' ? '随子模板提升' : '需保持关机'
  }

  const handleDelete = async () => {
    setSubmitting(true)
    try {
      const expectedVMs = relatedVMs.map((vm) => vm.name)
      const res = await deleteTemplate(node.name, {
        delete_mode: mode,
        delete_vms: !isPromoteMode && expectedVMs.length > 0,
        expected_vms: expectedVMs,
      })
      Toast.success(res.message || '删除模板任务已提交，请在任务中心查看进度')
      onDeleted()
      requestClose()
    } catch (err) {
      console.error('删除模板失败', err)
    } finally {
      setSubmitting(false)
    }
  }

  const templateColumns = [
    {
      title: '模板节点',
      dataIndex: 'name',
      render: (_: unknown, row: TemplateItem) => `${row.admin_name || row.name}（${row.name}）`,
    },
    { title: '用户侧显示', dataIndex: 'display_name' },
    {
      title: 'VM',
      dataIndex: 'direct_vm_count',
      width: 110,
      render: (_: unknown, row: TemplateItem) =>
        `${row.direct_vm_count || 0} / ${row.tree_vm_count || 0}`,
    },
  ]

  const vmColumns = [
    { title: '虚拟机名称', dataIndex: 'name' },
    { title: '来源模板', dataIndex: 'template' },
    ...(isPromoteMode
      ? [
          {
            title: '处理方式',
            dataIndex: 'status',
            render: (_: unknown, row: TemplateRelatedVM) => vmActionText(row),
          },
        ]
      : []),
    { title: '状态', dataIndex: 'status', width: 110 },
    { title: 'IP 地址', dataIndex: 'ip' },
  ]

  return (
    <Modal
      title="删除模板链路"
      visible={modalVisible}
      afterClose={afterModalClose}
      onCancel={requestClose}
      width={820}
      maskClosable={false}
      footer={
        <>
          <Button onClick={requestClose} disabled={submitting}>
            取消
          </Button>
          <Button
            type="danger"
            disabled={confirmDisabled || loading}
            loading={submitting}
            onClick={() => void handleDelete()}
          >
            {isPromoteMode ? '确认删除当前节点' : '确认删除链路'}
          </Button>
        </>
      }
    >
      <Spin spinning={loading} style={{ display: 'block' }}>
        <Banner
          type="warning"
          closeIcon={null}
          description={MODE_TIPS[mode]}
          style={{ marginBottom: 16 }}
        />

        <div className="qvm-form-item">
          <Radio.Group
            type="button"
            value={mode}
            onChange={(e) => setMode(e.target.value as TemplateDeleteMode)}
          >
            <Radio value="cascade">级联删除</Radio>
            <Radio value="promote_children" disabled={!parentTemplate}>
              仅删除当前节点并提升子节点
            </Radio>
            <Radio value="promote_children_hot" disabled={!parentTemplate}>
              热删除当前节点并提升子节点
            </Radio>
          </Radio.Group>
        </div>

        <Descriptions
          row
          size="small"
          style={{ marginBottom: 14 }}
          data={[
            { key: '起始模板', value: node.name || '-' },
            {
              key: '上级模板',
              value: parentTemplate
                ? parentTemplate.admin_name || parentTemplate.name
                : '无',
            },
            { key: '将删除节点', value: isPromoteMode ? 1 : templates.length },
            ...(isPromoteMode
              ? [
                  { key: '提升子模板', value: promotedTemplates.length },
                  { key: '重定向 VM', value: rebasedVMs.length },
                ]
              : []),
            { key: '关联虚拟机', value: relatedVMs.length },
          ]}
        />

        {isPromoteMode && blockers.length > 0 && (
          <Banner
            type="danger"
            closeIcon={null}
            description={blockers.join('；')}
            style={{ marginBottom: 14 }}
          />
        )}

        <Table
          columns={templateColumns}
          dataSource={isPromoteMode ? templates.slice(0, 1) : templates}
          rowKey="name"
          size="small"
          bordered
          pagination={false}
          scroll={{ y: 180 }}
          style={{ marginBottom: 14 }}
        />

        {isPromoteMode && promotedTemplates.length > 0 && (
          <Table
            columns={[
              {
                title: '将提升的子模板',
                dataIndex: 'name',
                render: (_: unknown, row: TemplateItem) =>
                  `${row.admin_name || row.name}（${row.name}）`,
              },
              {
                title: '新上级',
                dataIndex: 'parent',
                render: () => parentTemplate?.admin_name || parentTemplate?.name || '-',
              },
            ]}
            dataSource={promotedTemplates}
            rowKey="name"
            size="small"
            bordered
            pagination={false}
            scroll={{ y: 160 }}
            style={{ marginBottom: 14 }}
          />
        )}

        {relatedVMs.length > 0 && (
          <Table
            columns={vmColumns}
            dataSource={relatedVMs}
            rowKey="name"
            size="small"
            bordered
            pagination={false}
            scroll={{ y: 220 }}
          />
        )}
      </Spin>
    </Modal>
  )
}
