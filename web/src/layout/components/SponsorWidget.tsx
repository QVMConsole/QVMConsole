/**
 * 赞助支持入口（顶部导航栏）
 * - 图标按钮 + 下拉菜单：前往赞助 / 查看权益内容
 * - 赞助支持弹窗：首次访问当天不弹、次日起自动弹出；关闭后进入 7 天冷却期；
 *   弹出后 5 秒倒计时结束才允许关闭（与旧版前端逻辑保持一致）
 */
import { useEffect, useState } from 'react'
import { Modal, Dropdown, Button, Tooltip } from '@douyinfe/semi-ui'
import { IconLikeHeart, IconCoinMoneyStroked, IconArticle } from '@douyinfe/semi-icons'
import { EXTERNAL_LINKS, STORAGE_KEYS } from '@/config/constants'

/** 冷却期天数：关闭弹窗后 7 天内不再自动弹出 */
const SPONSOR_COOLDOWN_DAYS = 7
/** 弹窗强制阅读倒计时（秒） */
const SPONSOR_COUNTDOWN_SECONDS = 5

const openLink = (url: string) => {
  window.open(url, '_blank', 'noopener')
}

export default function SponsorWidget() {
  const [visible, setVisible] = useState(false)
  const [countdown, setCountdown] = useState(0)

  // 挂载时检查是否需要自动弹出赞助支持弹窗
  useEffect(() => {
    const today = new Date().toISOString().split('T')[0] // YYYY-MM-DD

    const firstVisit = localStorage.getItem(STORAGE_KEYS.sponsorFirstVisit)
    if (!firstVisit) {
      // 首次访问只记录日期，不打扰用户
      localStorage.setItem(STORAGE_KEYS.sponsorFirstVisit, today)
      return
    }
    // 仍在首次访问当天，不弹窗
    if (firstVisit === today) return

    // 检查冷却期：距上次关闭不足 7 天则跳过
    const lastClosed = localStorage.getItem(STORAGE_KEYS.sponsorLastClosed)
    if (lastClosed) {
      const daysSinceClosed = Math.floor((Date.now() - parseInt(lastClosed, 10)) / (1000 * 60 * 60 * 24))
      if (daysSinceClosed < SPONSOR_COOLDOWN_DAYS) return
    }

    setVisible(true)
    setCountdown(SPONSOR_COUNTDOWN_SECONDS)
  }, [])

  // 倒计时：弹窗可见且未结束时每秒递减
  useEffect(() => {
    if (!visible || countdown <= 0) return
    const timer = window.setTimeout(() => setCountdown((c) => c - 1), 1000)
    return () => window.clearTimeout(timer)
  }, [visible, countdown])

  const closeSponsor = () => {
    if (countdown > 0) return
    localStorage.setItem(STORAGE_KEYS.sponsorLastClosed, Date.now().toString())
    setVisible(false)
  }

  return (
    <>
      {/* 赞助下拉菜单（纯图标 + Tooltip） */}
      <Dropdown
        trigger="click"
        position="bottomRight"
        clickToHide
        render={
          <Dropdown.Menu>
            <Dropdown.Item icon={<IconCoinMoneyStroked />} onClick={() => openLink(EXTERNAL_LINKS.sponsorPay)}>
              前往赞助
            </Dropdown.Item>
            <Dropdown.Item icon={<IconArticle />} onClick={() => openLink(EXTERNAL_LINKS.sponsorBenefits)}>
              查看权益内容
            </Dropdown.Item>
          </Dropdown.Menu>
        }
      >
        <span className="qvm-sponsor-entry">
          <Tooltip content="赞助支持" position="bottom">
            <div className="qvm-tool-ic qvm-sponsor-btn">
              <IconLikeHeart />
            </div>
          </Tooltip>
        </span>
      </Dropdown>

      {/* 赞助支持弹窗 */}
      <Modal
        title="🤝 赞助支持 QVMConsole"
        visible={visible}
        width={480}
        closable={countdown <= 0}
        maskClosable={false}
        closeOnEsc={false}
        onCancel={closeSponsor}
        footer={
          <div className="qvm-sponsor-footer">
            {countdown > 0 && (
              <span className="qvm-sponsor-countdown-tip">请仔细阅读赞助权益，{countdown} 秒后可关闭</span>
            )}
            <Button disabled={countdown > 0} onClick={closeSponsor} className="qvm-sponsor-close-btn">
              {countdown > 0 ? `关闭 (${countdown}s)` : '关 闭'}
            </Button>
          </div>
        }
      >
        <div className="qvm-sponsor-content">
          <div className="qvm-sponsor-icon">
            <IconLikeHeart size="inherit" />
          </div>
          <h3 className="qvm-sponsor-title">喜欢 QVMConsole 吗？</h3>
          <p className="qvm-sponsor-desc">
            QVMConsole 是一个由个人开发者独立维护的开源 KVM 虚拟化管理面板。
            如果你觉得这个项目对你有帮助，欢迎赞助支持，帮助项目持续发展！
          </p>
          <div className="qvm-sponsor-benefits">
            <p className="qvm-sponsor-benefits-title">✨ 赞助者权益：</p>
            <ul className="qvm-sponsor-benefits-list">
              <li>优先技术支持响应</li>
              <li>功能需求优先排期</li>
              <li>赞助者专属身份标识</li>
              <li>内测版本优先体验</li>
            </ul>
          </div>
          <div className="qvm-sponsor-actions">
            <Button
              theme="solid"
              type="warning"
              icon={<IconCoinMoneyStroked />}
              onClick={() => openLink(EXTERNAL_LINKS.sponsorPay)}
            >
              前往赞助
            </Button>
            <Button icon={<IconArticle />} onClick={() => openLink(EXTERNAL_LINKS.sponsorBenefits)}>
              查看权益内容
            </Button>
          </div>
        </div>
      </Modal>
    </>
  )
}
