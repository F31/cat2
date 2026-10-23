import Taro, { useRouter } from '@tarojs/taro'
import { View, Text } from '@tarojs/components'
import { CONSENT_VERSION } from '../../utils/agreement'

const TERMS = `猫平方使用须知

一、平台定位
猫平方是面向个人与组织的商业合作服务平台，提供公开信息发布、主题圈子交流与猫立方协作工具。

二、账号与认证
1. 你应使用真实、合法的身份信息注册并使用平台。
2. 发布内容、创建圈子、创建猫立方及申请认证前，平台可能要求完成实名认证。
3. 通过认证的主体信息不得自行修改；确需变更的，请联系平台运营处理。

三、内容规范
1. 不得发布违法违规、侵犯他人权益、虚假宣传或恶意引流的内容。
2. 平台有权对违规内容进行下架、限制发布或封禁账号，并保留相关记录。
3. 圈内与猫立方内的内容仅限对应空间成员查看；分享到微信的内容以其落地页展示为准。

四、交易与会员
1. 会员与猫立方订阅的价格、权益以购买页面展示及平台配置为准。
2. 虚拟服务开通后不支持退款，法律法规另有规定的除外。
3. 支付、退款与发票相关事项以平台规则和订单页面为准。

五、其他
本须知为当前版本，正式上线前将由平台法务复核并更新。继续使用平台即表示你同意最新版本的使用须知。`

const PRIVACY = `猫平方隐私协议

一、我们收集的信息
1. 账号信息：微信授权标识、手机号、身份类型。
2. 资料信息：你主动填写的姓名、组织名称、区域、行业、合作需求与供给、简介等。
3. 认证信息：实名认证所需的姓名、证件号码及材料图片。
4. 使用信息：发布、关注、收藏、加入圈子、猫立方协作及消息记录。

二、我们如何使用信息
用于账号登录、匹配推荐、内容发布、认证审核、组织与圈子管理、猫立方协作、消息通知、安全风控与客户服务。

三、信息展示与共享
1. 你的名片与公开发布内容可能对其他用户展示；联系方式仅向符合会员权益的用户展示。
2. 认证材料仅用于审核，不对外公开展示。
3. 除法律法规要求或为完成服务所必需外，我们不会向第三方出售你的个人信息。

四、信息存储与保护
我们采取加密、访问控制、审计等措施保护个人信息，并在实现服务目的所需的期限内保存。

五、你的权利
你可以查看、更正个人资料，管理发布内容、关注与收藏，申请注销账号或联系平台客服处理个人信息相关请求。

六、联系我们
如对本协议有疑问，可通过小程序客服入口联系平台。正式上线前，本协议将由平台法务复核并更新。`

export default function Agreement() {
  const type = useRouter().params.type === 'privacy' ? 'privacy' : 'terms'
  Taro.setNavigationBarTitle({ title: type === 'privacy' ? '隐私协议' : '使用须知' })
  return (
    <View className="page">
      <View className="detail-block">
        <Text className="detail-block__body" style={{ whiteSpace: 'pre-wrap', lineHeight: 1.8 }}>
          {type === 'privacy' ? PRIVACY : TERMS}
        </Text>
      </View>
      <Text className="form-field__label" style={{ display: 'block', marginTop: '20px' }}>
        当前版本 v{CONSENT_VERSION}；正式上线前由法务确认，版本更新后将重新征得同意。
      </Text>
    </View>
  )
}
