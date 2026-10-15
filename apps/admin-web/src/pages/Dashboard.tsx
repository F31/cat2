import { useEffect, useState } from 'react'
import { Card, Col, Row, Statistic, message } from 'antd'
import { adminApi, type ReviewMetrics } from '../api/admin'

// 概览：审核队列 SLA 指标。
export default function Dashboard() {
  const [metrics, setMetrics] = useState<ReviewMetrics | null>(null)

  useEffect(() => {
    adminApi
      .reviewMetrics()
      .then(setMetrics)
      .catch((e) => message.error(e instanceof Error ? e.message : '加载失败'))
  }, [])

  return (
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
  )
}
