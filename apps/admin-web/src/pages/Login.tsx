import { useState } from 'react'
import { Button, Card, Form, Input, Typography, message } from 'antd'
import { useNavigate } from 'react-router-dom'
import { adminApi } from '../api/admin'

// 登录：后台手机号 + 密码（服务端 env 注入 PBKDF2 哈希；不再依赖开发态微信登录）。
export default function Login() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)

  const onFinish = async (v: { phone: string; password: string }) => {
    setLoading(true)
    try {
      await adminApi.loginPassword(v.phone, v.password)
      message.success('登录成功')
      navigate('/dashboard', { replace: true })
    } catch (e) {
      message.error(e instanceof Error ? e.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '100vh', background: '#f0f2f5' }}>
      <Card style={{ width: 360 }}>
        <Typography.Title level={4} style={{ textAlign: 'center' }}>
          猫平方管理后台
        </Typography.Title>
        <Form layout="vertical" onFinish={onFinish}>
          <Form.Item name="phone" label="手机号" rules={[{ required: true, message: '请输入手机号' }]}>
            <Input placeholder="11 位手机号" maxLength={11} />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password placeholder="后台密码" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登录
          </Button>
        </Form>
      </Card>
    </div>
  )
}
