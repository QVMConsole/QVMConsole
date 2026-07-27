/**
 * 工作台首页：按角色分发管理员 / 普通用户视图
 */
import { useUserStore } from '@/stores/user'
import { ROLES } from '@/config/constants'
import AdminDashboard from './AdminDashboard'
import UserDashboard from './UserDashboard'
import './dashboard.css'

export default function DashboardPage() {
  const role = useUserStore((s) => s.role)
  return role === ROLES.admin ? <AdminDashboard /> : <UserDashboard />
}
