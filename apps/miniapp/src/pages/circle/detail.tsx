import { useState } from 'react'
import Taro, { useDidShow, useRouter, useShareAppMessage } from '@tarojs/taro'
import { View, Text, Button, Canvas } from '@tarojs/components'
import type { Circle, CircleDemand, CircleFeed, CircleMember, CircleMount } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import StateView from '../../components/StateView'

const DEMAND_STATUS: Record<number, string> = { 0: '有效', 1: '已过期', 2: '已删除' }
const APPLY_LABEL: Record<number, string> = { 1: '审核中', 2: '已加入', 3: '未通过' }

// 圈详情：游客/非成员仅关于本圈+申请；成员看公告/动态/需求/挂载/成员；圈主可审核入圈申请。
export default function CircleDetail() {
  const router = useRouter()
  const id = router.params.id ?? ''
  const [circle, setCircle] = useState<Circle | null>(null)
  const [feeds, setFeeds] = useState<CircleFeed[]>([])
  const [demands, setDemands] = useState<CircleDemand[]>([])
  const [mounts, setMounts] = useState<CircleMount[]>([])
  const [members, setMembers] = useState<CircleMember[]>([])
  const [pendingCount, setPendingCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      const c = await api.circleDetail(id) // 游客可看
      setCircle(c)
      const joined = c.joined || (c.my_role ?? 0) > 0
      if (joined) {
        await ensureLogin()
        const [f, d, m, ms] = await Promise.all([
          api.listCircleFeeds(id),
          api.listCircleDemands(id),
          api.circleMounts(id),
          api.circleMembers(id),
        ])
        setFeeds(f.items ?? [])
        setDemands(d.items ?? [])
        setMounts(m.items ?? [])
        setMembers(ms.items ?? [])
        if (c.my_role === 1) {
          const apps = await api.listCircleApplications(id, 1)
          setPendingCount((apps.items ?? []).length)
        }
      }
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
    void Taro.showShareMenu({ withShareTicket: true })
  })

  useShareAppMessage(() => ({
    title: circle ? `来自「${circle.name}」的圈子` : '猫平方圈子',
    path: `/pages/circle/detail?id=${id}`,
  }))

  const saveCard = async () => {
    if (!circle) return
    try {
      Taro.showLoading({ title: '生成名片…' })
      const c = await api.circleCard(id)
      const fs = Taro.getFileSystemManager()
      const qrPath = `${Taro.env.USER_DATA_PATH}/circle-qr-${Date.now()}.png`
      fs.writeFileSync(qrPath, Taro.base64ToArrayBuffer(c.qr_png_base64), 'binary')
      const ctx = Taro.createCanvasContext('circleCard')
      ctx.setFillStyle('#ffffff')
      ctx.fillRect(0, 0, 750, 1000)
      ctx.setFillStyle('#2f54eb')
      ctx.fillRect(0, 0, 750, 12)
      ctx.setFillStyle('#111111')
      ctx.setFontSize(46)
      ctx.fillText(c.name.slice(0, 14), 40, 110)
      ctx.setFillStyle('#666666')
      ctx.setFontSize(26)
      wrapText(ctx, c.intro || '这个圈子还没有简介', 40, 175, 24, 38)
      ctx.setFillStyle('#333333')
      ctx.setFontSize(28)
      ctx.fillText(`圈主：${c.owner_name}`, 40, 470)
      ctx.fillText(`${c.member_count} 位成员`, 40, 520)
      ctx.setFillStyle('#2f54eb')
      ctx.setFontSize(30)
      ctx.fillText('扫码加入圈子', 40, 780)
      ctx.setFillStyle('#999999')
      ctx.setFontSize(22)
      ctx.fillText(`邀请码 ${c.invite_code}`, 40, 830)
      ctx.drawImage(qrPath, 470, 720, 220, 220)
      await new Promise<void>((resolve) => ctx.draw(false, () => resolve()))
      const res = await Taro.canvasToTempFilePath({ canvasId: 'circleCard' })
      await Taro.saveImageToPhotosAlbum({ filePath: res.tempFilePath })
      Taro.hideLoading()
      Taro.showToast({ title: '已保存到相册', icon: 'success' })
    } catch {
      Taro.hideLoading()
      Taro.showToast({ title: '保存失败（需相册权限）', icon: 'none' })
    }
  }

  const join = async () => {
    try {
      await ensureLogin()
      await api.applyCircle(id)
      Taro.showToast({ title: '已提交申请，等待圈主审核', icon: 'success' })
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 140003 ? '需先完成实名认证' : '申请失败', icon: 'none' })
    }
  }

  const share = async () => {
    try {
      await ensureLogin()
      const t = await api.circleInvite(id)
      Taro.setClipboardData({ data: t.code })
      Taro.showToast({ title: '邀请码已复制', icon: 'success' })
    } catch {
      Taro.showToast({ title: '仅成员可邀请', icon: 'none' })
    }
  }

  const renew = async (d: CircleDemand) => {
    try {
      await api.renewCircleDemand(d.id)
      Taro.showToast({ title: '已续期 30 天', icon: 'success' })
      void load()
    } catch {
      Taro.showToast({ title: '仅作者可续期', icon: 'none' })
    }
  }

  const toCube = async (d: CircleDemand) => {
    try {
      const res = await api.listCubes()
      const cubes = res.items ?? []
      if (cubes.length === 0) {
        Taro.showToast({ title: '请先创建或加入猫立方', icon: 'none' })
        return
      }
      const pick = await Taro.showActionSheet({ itemList: cubes.map((c) => c.name) })
      const cube = cubes[pick.tapIndex]
      if (!cube) return
      await api.demandToCube(id, d.id, cube.id)
      Taro.showToast({ title: '已转入猫立方办', icon: 'success' })
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 140002 ? '仅猫王/猫蜜可发起' : '转入失败', icon: 'none' })
    }
  }

  const openMount = (m: CircleMount) => {
    Taro.navigateTo({ url: `/pages/content/detail?ref=${encodeURIComponent(m.item_ref)}` })
  }

  if (!circle && (loading || error)) {
    return (
      <View className="page">
        <StateView loading={loading} error={error} onRetry={() => void load()} />
      </View>
    )
  }
  if (!circle) return null
  const joined = circle.joined || (circle.my_role ?? 0) > 0
  const isOwner = circle.my_role === 1

  return (
    <View className="page">
      <View className="detail-head">
        <Text className="detail-head__title">{circle.name}</Text>
        <Text className="detail-head__type">
          {circle.type === 1 ? '常设圈' : '临时圈'} · {circle.member_count} 人 · {circle.status === 0 ? '进行中' : circle.status === 1 ? '已结束' : '已解散'}
        </Text>
      </View>

      <View className="detail-block">
        <Text className="detail-block__title">关于本圈</Text>
        <Text className="detail-block__body">{circle.intro || '暂无简介'}</Text>
        {circle.region_code ? <Text className="form-field__label">区域：{circle.region_code}</Text> : null}
      </View>

      {!joined ? (
        <>
          {circle.apply_status === 1 ? (
            <View className="detail-block">
              <Text className="detail-block__title">申请已提交</Text>
              <Text className="detail-block__body">等待圈主审核，通过后会收到通知。</Text>
            </View>
          ) : null}
          {circle.apply_status === 3 ? (
            <View className="detail-block">
              <Text className="detail-block__title">上次申请未通过</Text>
              <Text className="detail-block__body">{circle.apply_reason || '可修改资料后重新申请。'}</Text>
            </View>
          ) : null}
          <View className="detail-actions">
            <Button className="btn-secondary" openType="share">
              分享圈子
            </Button>
            <Button className="btn-primary" disabled={circle.apply_status === 1} onClick={() => void join()}>
              {circle.apply_status === 1 ? '审核中' : circle.apply_status === 3 ? '重新申请' : '申请加入'}
            </Button>
          </View>
          <Text className="empty__hint">非成员仅可见「关于本圈」；加入后可查看动态、挂载与成员。</Text>
        </>
      ) : (
        <>
          {isOwner ? (
            <View className="detail-actions">
              <Button className="btn-secondary" onClick={() => Taro.navigateTo({ url: `/pages/circle/applications?id=${id}` })}>
                入圈申请{pendingCount > 0 ? `（${pendingCount}）` : ''}
              </Button>
              <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: `/pages/circle/publish?id=${id}&owner=${isOwner ? 1 : 0}` })}>
                发布公告 / 需求
              </Button>
            </View>
          ) : (
            <Button className="btn-primary" onClick={() => Taro.navigateTo({ url: `/pages/circle/publish?id=${id}&owner=${isOwner ? 1 : 0}` })}>
              发布需求 / 动态
            </Button>
          )}

          <View className="detail-block">
            <Text className="detail-block__title">圈内动态</Text>
            {feeds.map((f) => (
              <View key={f.id} className="menu__item">
                <View className="menu__row">
                  <Text>{f.author_name || f.author_id}</Text>
                  {f.is_announcement ? <Text className="row-tag">公告</Text> : null}
                </View>
                <Text className="detail-block__body">{f.text}</Text>
                <Text className="form-field__label">{f.created_at?.slice(0, 16).replace('T', ' ')}</Text>
              </View>
            ))}
            {feeds.length === 0 ? <Text className="form-field__label">暂无动态</Text> : null}
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">圈内需求</Text>
            {demands.map((d) => (
              <View key={d.id} className="menu__item">
                <View className="menu__row">
                  <Text>{d.title}</Text>
                  <Text className="row-tag">{d.transfer_ref ? '已转入猫立方' : DEMAND_STATUS[d.status] ?? d.status}</Text>
                </View>
                <Text className="detail-block__body">{d.content}</Text>
                <View className="menu__row-extra">
                  {d.status === 1 ? (
                    <Text className="row-action" onClick={() => void renew(d)}>续期</Text>
                  ) : null}
                  {!d.transfer_ref ? (
                    <Text className="row-action" onClick={() => void toCube(d)}>到猫立方办</Text>
                  ) : null}
                </View>
              </View>
            ))}
            {demands.length === 0 ? <Text className="form-field__label">暂无需求</Text> : null}
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">挂载内容（{mounts.length}）</Text>
            {mounts.map((m) => (
              <Text key={m.id} className="detail-block__body" onClick={() => openMount(m)}>
                {m.item_ref} {m.approval_status === 0 ? '（待确认）' : m.approval_status === 2 ? '（已拒绝）' : ''}
              </Text>
            ))}
            {mounts.length === 0 ? <Text className="form-field__label">暂无挂载</Text> : null}
          </View>

          <View className="detail-block">
            <Text className="detail-block__title">成员（{members.length}）</Text>
            {members.map((m) => (
              <Text key={m.user_id} className="detail-block__body">
                {m.nickname || `用户 ${m.user_id}`} · {m.role === 1 ? '圈主' : '成员'}
              </Text>
            ))}
          </View>

          <View className="detail-actions">
            <Button className="btn-secondary" onClick={() => void share()}>
              邀请成员
            </Button>
            <Button className="btn-secondary" onClick={() => void saveCard()}>
              保存圈名片
            </Button>
          </View>
          {isOwner ? (
            <Button className="btn-secondary" onClick={() => Taro.navigateTo({ url: `/pages/circle/settings?id=${id}` })}>
              圈设置
            </Button>
          ) : null}
        </>
      )}
      <Canvas canvasId="circleCard" id="circleCard" style={{ position: 'fixed', left: '-2000px', top: 0, width: '750px', height: '1000px' }} />
    </View>
  )
}

// 简单按字符换行（中文为主；最多 6 行）。
function wrapText(ctx: Taro.CanvasContext, text: string, x: number, y: number, perLine: number, lineHeight: number) {
  const chars = Array.from(text)
  let line = 0
  for (let i = 0; i < chars.length && line < 6; i += perLine) {
    ctx.fillText(chars.slice(i, i + perLine).join(''), x, y + line * lineHeight)
    line++
  }
}
