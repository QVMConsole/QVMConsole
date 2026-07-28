/**
 * 诊断导出 Tab：选择诊断类别后收集并导出 ZIP
 */
import { useEffect, useState } from 'react'
import { Banner, Button, Checkbox, CheckboxGroup, Spin, Toast } from '@douyinfe/semi-ui'
import { IconDownload, IconRefresh } from '@douyinfe/semi-icons'
import { exportDiagnostics, getDiagnosticCategories, type DiagnosticCategory } from '@/api/settings'
import { downloadBlob, timestampFilename } from '@/utils/download'

export default function DiagnosticsTab() {
  const [categories, setCategories] = useState<DiagnosticCategory[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [exporting, setExporting] = useState(false)

  useEffect(() => {
    setLoading(true)
    getDiagnosticCategories()
      .then((res) => {
        const list = res.data || []
        setCategories(list)
        // 默认全选
        setSelected(list.map((c) => c.id))
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const handleExport = async () => {
    if (selected.length === 0) {
      Toast.warning('请至少选择一个诊断类别')
      return
    }
    setExporting(true)
    try {
      const res = await exportDiagnostics({ categories: selected })
      downloadBlob(res.data, timestampFilename('qvmconsole-diagnostics', 'zip'))
      Toast.success('诊断信息导出成功')
    } catch {
      // 请求层已统一提示
    } finally {
      setExporting(false)
    }
  }

  return (
    <div className="stg-tab-pane">
      <Banner
        type="info"
        closeIcon={null}
        className="stg-banner"
        description="此功能收集系统及面板诊断信息用于排查问题。所有数据仅用于诊断分析，不会修改任何系统状态。"
      />

      <Spin spinning={loading}>
        <div className="stg-diag-toolbar">
          <Button
            size="small"
            theme="borderless"
            type="primary"
            onClick={() =>
              setSelected(
                selected.length === categories.length ? [] : categories.map((c) => c.id),
              )
            }
          >
            {selected.length === categories.length && categories.length > 0 ? '取消全选' : '全选'}
          </Button>
        </div>
        <CheckboxGroup
          value={selected}
          onChange={(v) => setSelected((v || []).map(String))}
          direction="vertical"
        >
          {categories.map((cat) => (
            <Checkbox key={cat.id} value={cat.id}>
              <span className="stg-diag-label">{cat.label}</span>
              {cat.description && <span className="stg-diag-desc">{cat.description}</span>}
            </Checkbox>
          ))}
        </CheckboxGroup>
      </Spin>

      <div className="stg-diag-actions">
        <Button
          type="primary"
          theme="solid"
          icon={exporting ? <IconRefresh spin /> : <IconDownload />}
          loading={exporting}
          disabled={selected.length === 0}
          onClick={() => void handleExport()}
        >
          收集并导出
        </Button>
        {exporting && <span className="stg-diag-exporting">正在收集诊断信息，请耐心等待...</span>}
      </div>
    </div>
  )
}
