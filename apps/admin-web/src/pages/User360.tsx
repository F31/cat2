import { useEffect, useState } from 'react'
import { Card, Col, Descriptions, Empty, Row, Spin, Tag, message } from 'antd'
import { useParams } from 'react-router-dom'
import { adminApi } from '../api/admin'

interface Section {
  module?: string
  [k: string]: unknown
}

// 用户 360（p4 §4.1）：只读聚合，每区标注来源模块；写操作走对应治理入口。
export default function User360() {
  const { id = '' } = useParams()
  const [sections, setSections] = useState<Record<string, Section>>({})
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    adminApi
      .user360(id)
      .then((r) => setSections((r.sections ?? {}) as Record<string, Section>))
      .catch((e) => message.error(e instanceof Error ? e.message : '加载失败'))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) return <Spin />
  if (Object.keys(sections).length === 0) return <Empty description="无数据" />

  const brief = (sections.identity?.brief ?? {}) as Record<string, unknown>
  const member = (sections.membership?.member ?? null) as Record<string, unknown> | null
  const certs = (sections.verify?.certs ?? []) as Array<Record<string, unknown>>
  const affs = (sections.verify?.affiliations ?? []) as Array<Record<string, unknown>>
  const items = (sections.content?.items ?? []) as Array<Record<string, unknown>>
  const cubes = (sections.cube?.cubes ?? []) as Array<Record<string, unknown>>
  const circles = (sections.circle?.circles ?? []) as Array<Record<string, unknown>>

  return (
    <>
      <Row gutter={16}>
        <Col span={12}>
          <Card title={`账号（identity） #${id}`}>
            <Descriptions column={1} size="small">
              <Descriptions.Item label="昵称">{(brief.nickname as string) || '—'}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag>{String(brief.status ?? '—')}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="身份类型">{String(brief.identity_type ?? '—')}</Descriptions.Item>
              <Descriptions.Item label="实名">{String(sections.identity?.realname_status ?? 0) === '1' ? <Tag color="green">已实名</Tag> : <Tag>未实名</Tag>}</Descriptions.Item>
              <Descriptions.Item label="受限">{sections.trust?.restricted ? <Tag color="red">限制中</Tag> : <Tag>正常</Tag>}</Descriptions.Item>
            </Descriptions>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="会员（membership）">
            {member ? (
              <Descriptions column={1} size="small">
                <Descriptions.Item label="等级">{String(member.level)}</Descriptions.Item>
                <Descriptions.Item label="到期">{String(member.expires_at ?? '—')}</Descriptions.Item>
              </Descriptions>
            ) : (
              <Empty description="非会员" />
            )}
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title={`认证与归属（verify）· ${certs.length} 项认证 / ${affs.length} 个单位`}>
            {certs.map((c, i) => (
              <div key={i}>
                <Tag>类型 {String(c.cert_type)}</Tag>
                <Tag color={c.status === 1 ? 'green' : c.status === 2 ? 'red' : 'default'}>{String(c.status)}</Tag>
                <span>{String(c.reject_reason ?? '')}</span>
              </div>
            ))}
            {certs.length === 0 ? <Empty description="无认证记录" /> : null}
          </Card>
        </Col>
        <Col span={12}>
          <Card title={`发布（content）· ${items.length} 条`}>
            {items.slice(0, 8).map((it, i) => (
              <div key={i}>
                <Tag>{String(it.kind)}</Tag>
                <span>{String(it.title)}</span>
              </div>
            ))}
            {items.length === 0 ? <Empty description="无发布" /> : null}
          </Card>
        </Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={12}>
          <Card title={`猫立方（cube）· ${cubes.length}`}>
            {cubes.map((c, i) => (
              <div key={i}>
                <Tag>角色 {String(c.role)}</Tag>
                <span>{String(((c.cube ?? {}) as Record<string, unknown>).name ?? '')}</span>
              </div>
            ))}
            {cubes.length === 0 ? <Empty description="无立方" /> : null}
          </Card>
        </Col>
        <Col span={12}>
          <Card title={`圈子（circle）· ${circles.length}`}>
            {circles.map((c, i) => (
              <div key={i}>
                <Tag>角色 {String(c.role)}</Tag>
                <span>{String(((c.circle ?? {}) as Record<string, unknown>).name ?? '')}</span>
              </div>
            ))}
            {circles.length === 0 ? <Empty description="无圈子" /> : null}
          </Card>
        </Col>
      </Row>
    </>
  )
}
