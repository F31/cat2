import type { ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { getToken } from './auth'
import AdminLayout from './layout/AdminLayout'
import Login from './pages/Login'
import Users from './pages/Users'
import Contents from './pages/Contents'
import Reviews from './pages/Reviews'
import Orders from './pages/Orders'
import Members from './pages/Members'
import Templates from './pages/Templates'
import DeadLetters from './pages/DeadLetters'
import FeatureFlags from './pages/FeatureFlags'
import Settings from './pages/Settings'
import AuditTrail from './pages/AuditTrail'
import Exports from './pages/Exports'
import Governance from './pages/Governance'
import AiPrompts from './pages/AiPrompts'
import Badges from './pages/Badges'
import Roles from './pages/Roles'
import User360 from './pages/User360'
import Dashboard from './pages/Dashboard'

function RequireAuth({ children }: { children: ReactNode }) {
  return getToken() ? <>{children}</> : <Navigate to="/login" replace />
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <AdminLayout />
          </RequireAuth>
        }
      >
        <Route index element={<Navigate to="/dashboard" replace />} />
        <Route path="dashboard" element={<Dashboard />} />
        <Route path="users" element={<Users />} />
        <Route path="contents" element={<Contents />} />
        <Route path="reviews" element={<Reviews />} />
        <Route path="orders" element={<Orders />} />
        <Route path="members" element={<Members />} />
        <Route path="templates" element={<Templates />} />
        <Route path="dead-letters" element={<DeadLetters />} />
        <Route path="feature-flags" element={<FeatureFlags />} />
        <Route path="settings" element={<Settings />} />
        <Route path="audits" element={<AuditTrail />} />
        <Route path="exports" element={<Exports />} />
        <Route path="governance" element={<Governance />} />
        <Route path="ai-prompts" element={<AiPrompts />} />
        <Route path="badges" element={<Badges />} />
        <Route path="roles" element={<Roles />} />
        <Route path="users/:id/360" element={<User360 />} />
      </Route>
    </Routes>
  )
}
