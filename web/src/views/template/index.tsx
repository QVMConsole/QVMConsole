/**
 * 模板管理页（深空极光版，仅管理员）
 * - 模板族卡片树形渲染（层级色条/展开箭头/链状摘要）
 * - 导入模板包（分片上传/秒传/断点续传 → 解析预览 → 确认导入）
 * - 发布设置（分类/默认创建配置/启动后命令）
 * - 删除模板链路（级联/提升子节点/热删除，高风险二次验证由请求层自动处理）
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button, Empty, Modal, Spin, Toast } from '@douyinfe/semi-ui'
import { IconChevronDown, IconChevronUp, IconRefresh, IconUpload } from '@douyinfe/semi-icons'
import { IllustrationNoContent, IllustrationNoContentDark } from '@douyinfe/semi-illustrations'
import {
  deleteTemplateExport,
  exportTemplate,
  getTemplateExportDownloadUrl,
  getLinuxTemplatePrepareCheck,
  getTemplateList,
  prepareImportedLinuxTemplate,
  type TemplateItem,
} from '@/api/template'
import { useUserStore } from '@/stores/user'
import { confirmModal } from '@/utils/confirm'
import { ROLES } from '@/config/constants'
import { buildTemplateTree, computeVisibleNodes, type TemplateTreeData } from './utils'
import type { TemplateNodeView } from './types'
import TemplateFamilyCard from './components/TemplateFamilyCard'
import type { TemplateNodeHandlers } from './components/TemplateNodeRow'
import ImportTemplateDialog from './dialogs/ImportTemplateDialog'
import PublishSettingsDialog from './dialogs/PublishSettingsDialog'
import DeleteTemplateChainDialog from './dialogs/DeleteTemplateChainDialog'
import './template.css'

/** 弹窗状态 */
type DialogState =
  | { type: 'import' }
  | { type: 'publish'; node: TemplateItem }
  | { type: 'delete'; node: TemplateItem }
  | null

export default function TemplateListPage() {
  const role = useUserStore((s) => s.role)
  const isAdmin = role === ROLES.admin

  const [items, setItems] = useState<TemplateItem[]>([])
  const [loading, setLoading] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const [expandState, setExpandState] = useState<Record<string, boolean>>({})
  const [exportingName, setExportingName] = useState('')
  const [deletingExportName, setDeletingExportName] = useState('')
  const [preparingLinuxName, setPreparingLinuxName] = useState('')
  const [dialog, setDialog] = useState<DialogState>(null)

  // ==================== 数据加载 ====================
  const fetchData = useCallback(async () => {
    setLoading(true)
    try {
      const res = await getTemplateList()
      setItems(res.data || [])
      setExpandState({}) // 刷新后默认全部收起
      setLoaded(true)
    } catch (err) {
      console.error('获取模板列表失败', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (isAdmin) void fetchData()
  }, [isAdmin, fetchData])

  // ==================== 族树构建与可见节点 ====================
  const treeData: TemplateTreeData = useMemo(() => buildTemplateTree(items), [items])

  const families = useMemo(
    () =>
      treeData.families.map((fam) => ({
        ...fam,
        visible_nodes: computeVisibleNodes(fam, treeData.childrenMap, expandState),
      })),
    [treeData, expandState],
  )

  // ==================== 展开/收起 ====================
  const toggleNode = useCallback((node: TemplateNodeView) => {
    setExpandState((prev) => ({ ...prev, [node.node_id]: !prev[node.node_id] }))
  }, [])

  const expandAll = useCallback(() => {
    setExpandState(() => {
      const next: Record<string, boolean> = {}
      items.forEach((n) => {
        if (n.node_id) next[n.node_id] = true
      })
      return next
    })
  }, [items])

  const collapseAll = useCallback(() => setExpandState({}), [])

  // ==================== 行内操作 ====================
  const handleExport = useCallback(async (node: TemplateItem, scope: 'root' | 'node') => {
    setExportingName(`${node.name}:${scope}`)
    try {
      const res = await exportTemplate(node.name, scope)
      Toast.success(res.message || '模板导出任务已提交')
    } catch (err) {
      console.error('导出模板失败', err)
    } finally {
      setExportingName('')
    }
  }, [])

  const handleDownloadExport = useCallback((node: TemplateItem) => {
    if (!node.export_path) {
      Toast.warning('导出包地址不存在，请重新导出')
      return
    }
    window.open(getTemplateExportDownloadUrl(node.export_path), '_blank')
  }, [])

  const handleDeleteExport = useCallback(
    async (node: TemplateItem) => {
      const ok = await confirmModal({
        title: '删除导出包',
        content: `确认删除模板「${node.admin_name || node.name}」的导出包？`,
        okText: '删除',
        danger: true,
      })
      if (!ok) return
      setDeletingExportName(node.name)
      try {
        const res = await deleteTemplateExport(node.name)
        Toast.success(res.message || '模板导出包已删除')
        void fetchData()
      } catch (err) {
        console.error('删除模板导出包失败', err)
      } finally {
        setDeletingExportName('')
      }
    },
    [fetchData],
  )

  const handlePrepareLinux = useCallback(async (node: TemplateItem) => {
    try {
      const checkRes = await getLinuxTemplatePrepareCheck(node.name)
      const check = checkRes.data
      if (check && !check.can_prepare) {
        const linkedVMs = check.linked_vms || []
        Modal.warning({
          title: '请先转换关联虚拟机',
          content: (
            <div className="tpl-prepare-blocker">
              <p>
                预处理会改写模板底盘。以下链式克隆虚拟机仍依赖该模板，继续操作会导致它们继承模板变更。
              </p>
              <ul>
                {linkedVMs.map((vm) => (
                  <li key={vm.name}>
                    <strong>{vm.name}</strong>
                    <span>（{vm.status || '状态未知'}）</span>
                  </li>
                ))}
              </ul>
              <p>
                请先关机，再前往“虚拟机管理”，在每台虚拟机的“更多”菜单中执行“转为独立虚拟机”；待全部转换任务完成后，再返回此处预处理。
              </p>
            </div>
          ),
          okText: '知道了',
        })
        return
      }
    } catch (err) {
      console.error('检查 Linux 模板链式依赖失败', err)
      return
    }

    const ok = await confirmModal({
      title: 'Linux 模板离线预处理',
      content: `将检查并补齐模板「${node.admin_name || node.name}」的 cloud-init 与磁盘扩容依赖。预处理会再次检查链式克隆依赖；若期间新增关联虚拟机，任务不会执行。`,
      okText: '提交任务',
    })
    if (!ok) return
    setPreparingLinuxName(node.name)
    try {
      const res = await prepareImportedLinuxTemplate(node.name)
      Toast.success(res.message || 'Linux 模板离线预处理任务已提交，请在任务中心查看进度')
    } catch (err) {
      console.error('提交 Linux 模板预处理失败', err)
    } finally {
      setPreparingLinuxName('')
    }
  }, [])

  const handlers: TemplateNodeHandlers = useMemo(
    () => ({
      onToggle: toggleNode,
      onExport: (node, scope) => void handleExport(node, scope),
      onDownloadExport: handleDownloadExport,
      onDeleteExport: (node) => void handleDeleteExport(node),
      onOpenPublish: (node) => setDialog({ type: 'publish', node }),
      onPrepareLinux: (node) => void handlePrepareLinux(node),
      onOpenDelete: (node) => setDialog({ type: 'delete', node }),
    }),
    [toggleNode, handleExport, handleDownloadExport, handleDeleteExport, handlePrepareLinux],
  )

  // ==================== 渲染 ====================
  if (!isAdmin) {
    return (
      <div className="tpl-page">
        <div className="tpl-empty">
          <div className="tpl-empty-icon">🔒</div>
          <div>模板管理仅对管理员开放</div>
        </div>
      </div>
    )
  }

  return (
    <div className="tpl-page">
      <div className="tpl-page-header qvm-fade-up">
        <h2>模板管理</h2>
        <div className="tpl-header-actions">
          <Button icon={<IconChevronDown />} onClick={expandAll}>
            全部展开
          </Button>
          <Button icon={<IconChevronUp />} onClick={collapseAll}>
            全部收起
          </Button>
          <Button
            icon={<IconUpload />}
            type="primary"
            theme="light"
            onClick={() => setDialog({ type: 'import' })}
          >
            导入模板包
          </Button>
          <Button icon={<IconRefresh />} loading={loading} onClick={() => void fetchData()}>
            刷新
          </Button>
        </div>
      </div>

      <Spin spinning={loading && !loaded} size="large" style={{ display: 'block' }}>
        <div className="tpl-family-list">
          {families.map((fam) => (
            <TemplateFamilyCard
              key={fam.template_uid}
              family={fam}
              byNodeId={treeData.byNodeId}
              childrenMap={treeData.childrenMap}
              expandState={expandState}
              exportingName={exportingName}
              deletingExportName={deletingExportName}
              preparingLinuxName={preparingLinuxName}
              handlers={handlers}
            />
          ))}
        </div>

        {loaded && families.length === 0 && (
          <Empty
            image={<IllustrationNoContent />}
            darkModeImage={<IllustrationNoContentDark />}
            title="暂无模板"
            description="可通过「虚拟机列表 → 更多 → 制作模板」创建首个模板"
            style={{ padding: '60px 0' }}
          />
        )}
      </Spin>

      {/* ==================== 弹窗 ==================== */}
      {dialog?.type === 'import' && (
        <ImportTemplateDialog onClose={() => setDialog(null)} onImported={() => void fetchData()} />
      )}
      {dialog?.type === 'publish' && (
        <PublishSettingsDialog
          node={dialog.node}
          onClose={() => setDialog(null)}
          onSaved={() => void fetchData()}
        />
      )}
      {dialog?.type === 'delete' && (
        <DeleteTemplateChainDialog
          node={dialog.node}
          onClose={() => setDialog(null)}
          onDeleted={() => void fetchData()}
        />
      )}
    </div>
  )
}
