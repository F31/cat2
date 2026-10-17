import { useEffect, useState } from 'react'
import { Card, Col, Row, Statistic, message, Tag } from 'antd'
import { adminApi, type ReviewMetrics } from '../api/admin'

// 概览：审核 SLA 指标 + 平台核心指标（口径见 docs/ops/dashboards.md，实时查询）。
export default function Dashboard() {
  const [metrics, setMetrics] = useState<ReviewMetrics | null>(null)
  const [core, setCore] = useState<Record<string, number>>({})
  const [groups, setGroups] = useState<Record<string, Record<string, number>>>({})
  const [funnel, setFunnel] = useState<Array<{ key: string; label: string; users: number; conversion: number }>>([])

  useEffect(() => {
    adminApi
      .reviewMetrics()
      .then(setMetrics)
      .catch((e) => message.error(e instanceof Error ? e.message : '加载失败'))
    adminApi
      .dashboard()
      .then((r) => {
        setCore(r.metrics ?? {})
        setGroups(r.sections ?? {})
      })
      .catch(() => undefined)
    adminApi
      .dashboardFunnel(7)
      .then((r) => setFunnel(r.steps ?? []))
      .catch(() => undefined)
  }, [])

  const money = (fen?: number) => ((fen ?? 0) / 100).toFixed(2)

  return (
    <>
      <Row gutter={16}>
        <Col span={8}>
          <Card>
            <Statistic title="待审任务" value={metrics?.pending ?? 0} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="超 SLA 任务" value={metrics?.overdue ?? 0} valueStyle={{ color: '#cf1322' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic title="平均复核耗时（秒）" value={metrics?.avg_review_seconds ?? 0} />
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={6}>
          <Card>
            <Statistic title="用户总数" value={core.users_total ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="今日新增用户" value={core.users_new_today ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="已实名用户" value={core.realname_users ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="在有效期会员" value={core.members_active ?? 0} />
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={6}>
          <Card>
            <Statistic title="今日支付订单" value={core.orders_paid_today ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="今日 GMV（元）" value={money(core.gmv_fen_today)} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="活跃圈子" value={core.circles_active ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="活跃猫立方" value={core.cubes_active ?? 0} />
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={6}>
          <Card>
            <Statistic title="今日搜索" value={core.search_events_today ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="今日匹配点击" value={core.feed_clicks_today ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="今日联系发起" value={core.contacts_today ?? 0} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="AI 调用/降级（今日）" value={`${core.ai_calls_today ?? 0} / ${core.ai_degraded_today ?? 0}`} />
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title="漏斗（近 7 天，去重用户）">
            {funnel.map((s) => (
              <div key={s.key} style={{ marginBottom: 8 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <span>{s.label}</span>
                  <span>
                    {s.users}（{(s.conversion * 100).toFixed(1)}%）
                  </span>
                </div>
                <div style={{ height: 8, background: '#303030', borderRadius: 4 }}>
                  <div
                    style={{
                      width: `${Math.min(100, s.conversion * 100).toFixed(1)}%`,
                      height: 8,
                      background: '#1677ff',
                      borderRadius: 4,
                    }}
                  />
                </div>
              </div>
            ))}
            {funnel.length === 0 ? <span>暂无埋点数据</span> : null}
          </Card>
        </Col>
        <Col span={12}>
          <Card title="协作 / 资金（实时）">
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
              <span>活跃圈 {groups.collaboration?.circles_active ?? 0}</span>
              <span>活跃立方 {groups.collaboration?.cubes_active ?? 0}</span>
              <span>进行中需求 {groups.collaboration?.demands_active ?? 0}</span>
              <span>分摊进行中 {groups.collaboration?.apportion_open ?? 0}</span>
              <span>今日退款笔数 {groups.funds?.refunds_today ?? 0}</span>
              <span>今日退款（元）{money(groups.funds?.refund_fen_today)}</span>
              <span>履约异常 {groups.funds?.fulfillment_stuck ?? 0}</span>
              <span>24h 拒绝审计 {groups.security?.denied_24h ?? 0}</span>
            </div>
          </Card>
        </Col>
      </Row>
      <div style={{ marginTop: 12 }}>
        <Tag color={core.outbox_dead ? 'red' : 'green'}>死信 {core.outbox_dead ?? 0}</Tag>
        <Tag color={core.review_overdue ? 'red' : 'green'}>审核超 SLA {core.review_overdue ?? 0}</Tag>
        <Tag>内容展示中 {core.contents_published ?? 0}</Tag>
      </div>
    </>
  )
}
