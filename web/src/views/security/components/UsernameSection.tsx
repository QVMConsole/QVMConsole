/**
 * 修改用户名区块
 * - 需输入当前密码确认身份；成功后后端重新签发 Token，本地同步更新
 */
import { useState } from 'react'
import { Button, Input, Toast } from '@douyinfe/semi-ui'
import { changeUsername } from '@/api/auth'
import { useUserStore } from '@/stores/user'

export default function UsernameSection() {
  const username = useUserStore((s) => s.username)

  const [form, setForm] = useState({ newUsername: '', password: '' })
  const [loading, setLoading] = useState(false)

  const handleSubmit = async () => {
    const newUsername = form.newUsername.trim()
    if (!newUsername) {
      Toast.warning('请输入新用户名')
      return
    }
    if (newUsername.length < 3 || newUsername.length > 32) {
      Toast.warning('用户名长度需在 3-32 个字符之间')
      return
    }
    if (newUsername === username) {
      Toast.warning('新用户名与当前用户名相同')
      return
    }
    if (!form.password) {
      Toast.warning('请输入密码以确认身份')
      return
    }

    setLoading(true)
    try {
      const res = await changeUsername({ new_username: newUsername, password: form.password })
      const { token, username: updated } = res.data
      // 后端重新签发 Token，同步更新本地登录态
      const store = useUserStore.getState()
      store.setToken(token)
      store.setUserInfo(updated, store.role, store.security, store.cloudType)
      Toast.success(res.message || '用户名修改成功')
      setForm({ newUsername: '', password: '' })
    } catch {
      // 请求层已统一提示
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="sec-tab-pane">
      <div className="sec-row">
        <div className="sec-row-label">当前用户名</div>
        <div className="sec-row-main">
          <Input value={username} disabled />
        </div>
      </div>
      <div className="sec-row">
        <div className="sec-row-label">新用户名</div>
        <div className="sec-row-main">
          <Input
            value={form.newUsername}
            onChange={(v) => setForm((p) => ({ ...p, newUsername: v }))}
            maxLength={32}
            placeholder="请输入新用户名（3-32 个字符）"
          />
        </div>
      </div>
      <div className="sec-row">
        <div className="sec-row-label">确认密码</div>
        <div className="sec-row-main">
          <Input
            mode="password"
            value={form.password}
            onChange={(v) => setForm((p) => ({ ...p, password: v }))}
            placeholder="请输入密码以确认身份"
          />
        </div>
      </div>
      <div className="sec-row">
        <div className="sec-row-label" />
        <div className="sec-row-main sec-actions">
          <Button type="primary" theme="solid" loading={loading} onClick={() => void handleSubmit()}>
            确认修改
          </Button>
        </div>
      </div>
    </div>
  )
}
