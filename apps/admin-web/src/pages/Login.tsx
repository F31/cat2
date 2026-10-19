import { useEffect, useState } from 'react'
import { Button, Checkbox, ConfigProvider, Form, Input, Modal, message, theme } from 'antd'
import { useNavigate } from 'react-router-dom'
import { adminApi } from '../api/admin'
import './Login.css'

const REMEMBER_KEY = 'catcube.admin.phone'

type LoginForm = { phone: string; password: string }
type ResetForm = { phone: string; code: string; new_password: string; confirm_password: string }

const maskPhone = (phone: string) => (phone.length === 11 ? `${phone.slice(0, 3)}****${phone.slice(7)}` : phone)

// 登录：后台手机号 + 密码（服务端 env/DB 注入 PBKDF2 哈希）。
export default function Login() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [remember, setRemember] = useState(false)
  const [forgotOpen, setForgotOpen] = useState(false)
  const [sendingCode, setSendingCode] = useState(false)
  const [resetLoading, setResetLoading] = useState(false)
  const [countdown, setCountdown] = useState(0)
  const [sentPhone, setSentPhone] = useState('')
  const [form] = Form.useForm<LoginForm>()
  const [resetForm] = Form.useForm<ResetForm>()

  useEffect(() => {
    const saved = localStorage.getItem(REMEMBER_KEY)
    if (saved) {
      setRemember(true)
      form.setFieldsValue({ phone: saved })
    }
  }, [form])

  useEffect(() => {
    if (countdown <= 0) return
    const timer = window.setTimeout(() => setCountdown((n) => n - 1), 1000)
    return () => window.clearTimeout(timer)
  }, [countdown])

  const openForgot = () => {
    resetForm.resetFields()
    const phone = form.getFieldValue('phone') || localStorage.getItem(REMEMBER_KEY) || ''
    if (phone) resetForm.setFieldsValue({ phone })
    setSentPhone('')
    setCountdown(0)
    setForgotOpen(true)
  }

  const sendResetCode = async () => {
    try {
      await resetForm.validateFields(['phone'])
    } catch {
      return
    }
    const phone = String(resetForm.getFieldValue('phone') || '')
    setSendingCode(true)
    try {
      await adminApi.resetPasswordCode(phone)
      setSentPhone(phone)
      setCountdown(60)
      message.success('验证码已发送，请注意查收')
    } catch (e) {
      message.error(e instanceof Error ? e.message : '验证码发送失败')
    } finally {
      setSendingCode(false)
    }
  }

  const submitReset = async (v: ResetForm) => {
    setResetLoading(true)
    try {
      await adminApi.resetPassword(v.phone, v.code, v.new_password)
      message.success('密码已重置，请使用新密码登录')
      setForgotOpen(false)
      setSentPhone('')
      setCountdown(0)
      resetForm.resetFields()
      form.setFieldsValue({ phone: v.phone })
    } catch (e) {
      message.error(e instanceof Error ? e.message : '密码重置失败')
    } finally {
      setResetLoading(false)
    }
  }

  const onFinish = async (v: LoginForm) => {
    setLoading(true)
    try {
      await adminApi.loginPassword(v.phone, v.password)
      if (remember) localStorage.setItem(REMEMBER_KEY, v.phone)
      else localStorage.removeItem(REMEMBER_KEY)
      message.success('登录成功')
      navigate('/dashboard', { replace: true })
    } catch (e) {
      message.error(e instanceof Error ? e.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-page">
      <section className="login-visual">
        <div className="login-visual-inner">
          <img className="login-brand" src="/brand-logo.png" alt="猫平方 CAT² CONSOLE" />
          <div className="login-copy">
            <h1>
              助力城市商业服务中心，
              <br />
              打造自主掌控的渠道体系
            </h1>
            <p>猫平方商业一体化综合服务平台 · 总管理后台</p>
          </div>
        </div>
        <div className="login-shape login-shape-lg" />
        <div className="login-shape login-shape-sm" />
        <div className="login-foot">© 猫平方</div>
      </section>

      <section className="login-panel">
        <ConfigProvider
          theme={{
            algorithm: theme.defaultAlgorithm,
            token: {
              colorPrimary: '#1f6f6b',
              colorText: '#102a2e',
              colorTextSecondary: '#5f6f6f',
              colorBgContainer: '#ffffff',
              colorBorder: '#d7dedc',
              borderRadius: 6,
              controlHeightLG: 52,
              fontSize: 16,
              fontFamily: '"PingFang SC", "Microsoft YaHei", "Noto Sans SC", system-ui, sans-serif',
            },
            components: {
              Button: { controlHeightLG: 54, fontWeight: 600 },
              Input: { activeShadow: '0 0 0 2px rgba(31,111,107,.12)' },
            },
          }}
        >
          <div className="login-form-wrap">
            <header className="login-form-head">
              <h2>登录管理后台</h2>
              <p>请使用运营账号登录，所有操作均记录审计日志。</p>
            </header>

            <Form form={form} layout="vertical" requiredMark={false} onFinish={onFinish} size="large">
              <Form.Item name="phone" label="账号" rules={[{ required: true, message: '请输入账号' }]}>
                <Input placeholder="请输入运营账号" maxLength={11} inputMode="numeric" autoComplete="username" />
              </Form.Item>
              <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
                <Input.Password placeholder="请输入密码" autoComplete="current-password" />
              </Form.Item>

              <div className="login-options">
                <Checkbox checked={remember} onChange={(e) => setRemember(e.target.checked)}>
                  记住账号
                </Checkbox>
                <button
                  type="button"
                  className="login-link"
                  onClick={openForgot}
                >
                  忘记密码？
                </button>
              </div>

              <Button type="primary" htmlType="submit" block loading={loading} className="login-submit">
                登录
              </Button>
            </Form>

            <div className="login-legal">
              <span className="login-legal-line" />
              <p>仅限授权运营人员访问，所有操作将记录审计日志。</p>
            </div>
          </div>

          <Modal
            className="login-reset-modal"
            title="找回密码"
            open={forgotOpen}
            onCancel={() => {
              setForgotOpen(false)
              setSentPhone('')
              setCountdown(0)
              resetForm.resetFields()
            }}
            onOk={() => resetForm.submit()}
            confirmLoading={resetLoading}
            okText="重置密码"
            cancelText="取消"
            forceRender
          >
            <p className="login-reset-tip">通过账号绑定手机获取短信验证码，验证后设置新密码。</p>
            <Form form={resetForm} layout="vertical" requiredMark={false} onFinish={submitReset} size="large">
              <Form.Item
                name="phone"
                label="账号"
                rules={[
                  { required: true, message: '请输入账号' },
                  { pattern: /^1\d{10}$/, message: '请输入 11 位手机号' },
                ]}
              >
                <Input placeholder="请输入运营账号" maxLength={11} inputMode="numeric" autoComplete="username" />
              </Form.Item>
              <Form.Item
                name="code"
                label="短信验证码"
                rules={[
                  { required: true, message: '请输入验证码' },
                  { pattern: /^\d{6}$/, message: '请输入 6 位数字验证码' },
                ]}
              >
                <Input
                  placeholder="6 位数字"
                  maxLength={6}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  addonAfter={
                    <Button
                      type="link"
                      size="small"
                      className="login-code-btn"
                      htmlType="button"
                      disabled={countdown > 0}
                      loading={sendingCode}
                      onClick={() => void sendResetCode()}
                    >
                      {countdown > 0 ? `${countdown}s 后重发` : '获取验证码'}
                    </Button>
                  }
                />
              </Form.Item>
              {sentPhone ? <div className="login-reset-hint">验证码已发送至 {maskPhone(sentPhone)}</div> : null}
              <Form.Item
                name="new_password"
                label="新密码"
                rules={[
                  { required: true, message: '请输入新密码' },
                  { min: 8, message: '新密码至少 8 位' },
                  { max: 128, message: '新密码过长' },
                ]}
              >
                <Input.Password autoComplete="new-password" placeholder="至少 8 位" />
              </Form.Item>
              <Form.Item
                name="confirm_password"
                label="确认新密码"
                dependencies={['new_password']}
                rules={[
                  { required: true, message: '请再次输入新密码' },
                  ({ getFieldValue }) => ({
                    validator(_, value) {
                      if (!value || getFieldValue('new_password') === value) return Promise.resolve()
                      return Promise.reject(new Error('两次输入的新密码不一致'))
                    },
                  }),
                ]}
              >
                <Input.Password autoComplete="new-password" placeholder="再次输入新密码" />
              </Form.Item>
            </Form>
          </Modal>
        </ConfigProvider>
      </section>
    </div>
  )
}
