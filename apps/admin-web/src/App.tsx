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
      </Route>
    </Routes>
  )
}
