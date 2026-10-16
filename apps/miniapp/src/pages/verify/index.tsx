import { useState } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import type { Cert, Affiliation } from '@catcube/api-client'
import { api } from '../../services/request'
import { ensureLogin } from '../../services/auth'
import FormField from '../../components/FormField'
import Uploader from '../../components/Uploader'
import StateView from '../../components/StateView'

// 猫²认证（p4 §6.9）：
// 1) 实名（1）：姓名 + 身份证号 + 协议，第三方二要素实时核验（免证件照）。
// 2) 其他四类（2 个人资质 / 3 个人企业身份 / 4 企业 / 5 机构）：材料多图（每类 ≤3）+ 说明，
//    3 类由所选企业管理员审，2/4/5 类由平台审。
// 3) 单位归属（一人多单位）：申请 → 组织管理员核验（未核验置灰、已核验红框）；可设主单位。
// 4) 组织管理员：查看本组织管理员列表与待核验归属（红点），核验处理。
const CERT_LABELS: Record<number, string> = {
  1: '实名认证',
  2: '个人资质认证',
  3: '个人企业身份',
  4: '企业认证',
  5: '机构认证',
}
const CERT_HINT: Record<number, string> = {
  2: '上传专业资质/证书图片（≤3），平台审核 1-3 个工作日',
  3: '选择已入驻企业并上传任职证明（≤3），由该企业管理员审核',
  4: '上传营业执照与法人身份证（≤3），平台审核',
  5: '上传证件/批文、负责人身份证明、授权书（≤3），平台审核',
}
const STATUS_TEXT: Record<number, string> = { 0: '待审核', 1: '已通过', 2: '已驳回', 3: '已撤销' }
const AFF_STATUS: Record<number, string> = { 0: '待核验', 1: '已核验', 2: '已驳回', 3: '已撤销' }

export default function Verify() {
  const [certs, setCerts] = useState<Cert[]>([])
  const [affs, setAffs] = useState<Affiliation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)

  // 实名表单
  const [name, setName] = useState('')
  const [idNo, setIdNo] = useState('')
  const [agreed, setAgreed] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  // 其他认证表单（当前选中类型）
  const [kind, setKind] = useState<number>(0)
  const [materials, setMaterials] = useState<string[]>([])
  const [extra, setExtra] = useState('')
  const [targetOrg, setTargetOrg] = useState('')

  // 单位归属表单
  const [affOrg, setAffOrg] = useState('')
  const [affRole, setAffRole] = useState('')
  const [affProof, setAffProof] = useState<string[]>([])
  // 组织管理员视角：待核验归属
  const [adminOrg, setAdminOrg] = useState('')
  const [pending, setPending] = useState<Affiliation[]>([])

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      await ensureLogin()
      const res = await api.myCerts()
      setCerts(res.items ?? [])
      try {
        const a = await api.myAffiliations()
        setAffs(a.items ?? [])
      } catch {
        setAffs([])
      }
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  const realname = certs.find((c) => c.cert_type === 1) ?? null
  const passed = realname?.status === 1
  const rejected = realname?.status === 2

  const submitRealname = async () => {
    if (!name.trim() || !idNo.trim()) {
      Taro.showToast({ title: '请填写姓名与身份证号', icon: 'none' })
      return
    }
    if (!agreed) {
      Taro.showToast({ title: '请先勾选实名认证服务协议', icon: 'none' })
      return
    }
    setSubmitting(true)
    try {
      await ensureLogin()
      const res = await api.submitCert({ cert_type: 1, real_name: name.trim(), id_card_no: idNo.trim(), agreed })
      Taro.showToast({ title: res.status === 1 ? '实名认证通过' : '已提交审核', icon: 'success' })
      void load()
    } catch (e) {
      const code = e && typeof e === 'object' && 'code' in e ? (e as { code: number }).code : 0
      Taro.showToast({ title: code === 100002 ? '姓名与证件号不一致' : '认证失败，请核对信息', icon: 'none' })
    } finally {
      setSubmitting(false)
    }
  }

  const submitMaterialCert = async () => {
    if (materials.length === 0) {
      Taro.showToast({ title: '请上传材料（每类最多 3 项）', icon: 'none' })
      return
    }
    if (kind === 3 && !targetOrg.trim()) {
      Taro.showToast({ title: '请填写目标企业账号 ID', icon: 'none' })
      return
    }
    setSubmitting(true)
    try {
      await ensureLogin()
      await api.submitCert({
        cert_type: kind,
        agreed: true,
        material_file_ids: materials,
        target_org_id: kind === 3 ? targetOrg.trim() : undefined,
        extra: extra ? { note: extra } : undefined,
      })
      Taro.showToast({ title: '已提交，等待审核', icon: 'success' })
      setKind(0)
      setMaterials([])
      setExtra('')
      setTargetOrg('')
      void load()
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '提交失败', icon: 'none' })
    } finally {
      setSubmitting(false)
    }
  }

  const applyAffiliation = async () => {
    if (!affOrg.trim()) {
      Taro.showToast({ title: '请填写组织账号 ID', icon: 'none' })
      return
    }
    setSubmitting(true)
    try {
      await ensureLogin()
      await api.applyAffiliation(affOrg.trim(), affRole.trim(), affProof)
      Taro.showToast({ title: '已提交，等待组织管理员核验', icon: 'success' })
      setAffOrg('')
      setAffRole('')
      setAffProof([])
      void load()
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '提交失败', icon: 'none' })
    } finally {
      setSubmitting(false)
    }
  }

  const setPrimary = async (id: string) => {
    try {
      await api.setPrimaryAffiliation(id)
      Taro.showToast({ title: '已设为主单位', icon: 'success' })
      void load()
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '操作失败', icon: 'none' })
    }
  }

  const loadPending = async () => {
    if (!adminOrg.trim()) {
      Taro.showToast({ title: '请填写你管理的组织账号 ID', icon: 'none' })
      return
    }
    try {
      const res = await api.pendingAffiliations(adminOrg.trim())
      setPending(res.items ?? [])
      Taro.showToast({ title: `待核验 ${(res.items ?? []).length} 条`, icon: 'none' })
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '无权限或加载失败', icon: 'none' })
    }
  }

  const verifyAff = async (id: string, pass: boolean) => {
    try {
      await api.verifyAffiliation(id, pass, pass ? '' : '材料不足')
      Taro.showToast({ title: pass ? '已核验' : '已驳回', icon: 'success' })
      void loadPending()
    } catch (e) {
      Taro.showToast({ title: e instanceof Error ? e.message : '处理失败', icon: 'none' })
    }
  }

  return (
    <View className="page">
      <StateView loading={loading} error={error} onRetry={() => void load()}>
        {/* 1. 实名 */}
        {passed ? (
          <View className="detail-block">
            <Text className="detail-block__title">已实名认证</Text>
            <Text className="detail-block__body">你已完成实名认证，可发布内容、创建圈子与猫立方。</Text>
          </View>
        ) : (
          <>
            <View className="detail-block">
              <Text className="detail-block__title">实名认证</Text>
              <Text className="detail-block__body">
                仅用于发布内容、创建圈子与猫立方等操作；浏览、搜索、查看详情无需实名。
              </Text>
            </View>
            {rejected ? (
              <View className="policy-gate">
                <Text className="policy-gate__hint">上次认证未通过：{realname?.reject_reason || '信息不一致'}</Text>
              </View>
            ) : null}
            <FormField label="真实姓名" required value={name} onChange={setName} />
            <FormField label="身份证号" required value={idNo} onChange={setIdNo} />
            <View className="agreement" onClick={() => setAgreed(!agreed)}>
              <Text className={agreed ? 'checkbox checkbox--on' : 'checkbox'}>{agreed ? '✓' : ''}</Text>
              <Text className="agreement__text">我已阅读并同意《实名认证服务协议》</Text>
            </View>
            <Button className="btn-primary" loading={submitting} onClick={() => void submitRealname()}>
              提交认证
            </Button>
          </>
        )}

        {/* 2. 其他认证类型 */}
        <View className="detail-block">
          <Text className="detail-block__title">更多认证</Text>
          <Text className="detail-block__body">个人 / 个人企业身份 / 企业 / 机构，材料每类最多 3 项。</Text>
        </View>
        {certs
          .filter((c) => c.cert_type !== 1)
          .map((c) => (
            <View key={String(c.id)} className="list-row">
              <Text className="list-row__title">{CERT_LABELS[c.cert_type] ?? `类型 ${c.cert_type}`}</Text>
              <Text className="list-row__meta">{STATUS_TEXT[c.status] ?? c.status}</Text>
            </View>
          ))}
        <View className="tabs">
          {[2, 3, 4, 5].map((k) => (
            <Text
              key={k}
              className={kind === k ? 'tabs__item tabs__item--active' : 'tabs__item'}
              onClick={() => setKind(kind === k ? 0 : k)}
            >
              {CERT_LABELS[k]}
            </Text>
          ))}
        </View>
        {kind !== 0 ? (
          <>
            <Text className="detail-block__body">{CERT_HINT[kind]}</Text>
            {kind === 3 ? (
              <FormField label="目标企业账号 ID" required value={targetOrg} onChange={setTargetOrg} />
            ) : null}
            <Uploader label="认证材料" max={3} value={materials} onChange={setMaterials} />
            <FormField label="补充说明（可选）" value={extra} onChange={setExtra} />
            <Button className="btn-primary" loading={submitting} onClick={() => void submitMaterialCert()}>
              提交{kind === 3 ? '企业审核' : '平台审核'}
            </Button>
          </>
        ) : null}

        {/* 3. 单位归属（一人多单位） */}
        <View className="detail-block">
          <Text className="detail-block__title">单位归属（一人多单位）</Text>
          <Text className="detail-block__body">核验通过后展示红框标识；未核验置灰。可设唯一主单位。</Text>
        </View>
        {affs.map((a) => (
          <View key={String(a.id)} className="list-row">
            <Text className={a.status === 1 ? 'list-row__title list-row__title--verified' : 'list-row__title'}>
              {`组织 ${a.org_id}${a.role ? ` · ${a.role}` : ''}${a.is_primary ? ' · 主单位' : ''}`}
            </Text>
            <Text className="list-row__meta">{AFF_STATUS[a.status] ?? a.status}</Text>
            {a.status === 1 && !a.is_primary ? (
              <Button size="mini" className="btn-secondary" onClick={() => void setPrimary(String(a.id))}>
                设为主单位
              </Button>
            ) : null}
          </View>
        ))}
        <FormField label="组织账号 ID" value={affOrg} onChange={setAffOrg} />
        <FormField label="职务 / 角色（可选）" value={affRole} onChange={setAffRole} />
        <Uploader label="任职证明（可选）" max={3} value={affProof} onChange={setAffProof} />
        <Button className="btn-secondary" loading={submitting} onClick={() => void applyAffiliation()}>
          申请加入组织
        </Button>

        {/* 4. 组织管理员核验（管理员视角：红点提醒） */}
        <View className="detail-block">
          <Text className="detail-block__title">待我核验的归属</Text>
          <Text className="detail-block__body">组织管理员先到先处理；超 7 天平台可代处理。</Text>
        </View>
        <FormField label="我管理的组织账号 ID" value={adminOrg} onChange={setAdminOrg} />
        <Button className="btn-secondary" onClick={() => void loadPending()}>
          刷新待核验（红点）
        </Button>
        {pending.length === 0 ? (
          <Text className="detail-block__body">暂无待核验</Text>
        ) : (
          pending.map((a) => (
            <View key={`v-${String(a.id)}`} className="list-row">
              <Text className="list-row__title">{`用户 ${a.user_id} 申请加入（${a.role || '未填职务'}）`}</Text>
              <View className="list-row__actions">
                <Button size="mini" className="btn-primary" onClick={() => void verifyAff(String(a.id), true)}>
                  通过
                </Button>
                <Button size="mini" className="btn-secondary" onClick={() => void verifyAff(String(a.id), false)}>
                  驳回
                </Button>
              </View>
            </View>
          ))
        )}
      </StateView>
    </View>
  )
}
