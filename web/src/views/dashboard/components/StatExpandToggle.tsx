/**
 * 统计卡「硬件详情」折叠开关（概览页 CPU / 内存卡片共用）
 * - 文本 + 旋转 chevron，默认收起；点击切换展开状态
 */
interface StatExpandToggleProps {
  open: boolean
  onToggle: () => void
  label?: string
}

export default function StatExpandToggle({ open, onToggle, label = '硬件详情' }: StatExpandToggleProps) {
  return (
    <button
      type="button"
      className={`qvm-stat-expand-btn${open ? ' open' : ''}`}
      onClick={onToggle}
      aria-expanded={open}
    >
      <span>{label}</span>
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path
          d="M6 9l6 6 6-6"
          stroke="currentColor"
          strokeWidth="2.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </button>
  )
}
