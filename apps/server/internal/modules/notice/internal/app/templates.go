// templates.go 通知模板登记表（PRD 11.3 触达逐条映射；业务模块只发事件，文案集中在此）。
package app

import "catcube/internal/modules/notice/internal/domain"

// Templates 全量模板（后台可改文案/渠道；closeable=false 为资金与审核类不可关闭）。
var Templates = []domain.Template{
	// identity / profile / verify
	{Code: "identity.UserRegistered.v1", Module: "identity", EventCode: "identity.UserRegistered.v1", TitleTpl: "欢迎加入猫平方", ContentTpl: "完善名片可提升匹配度，祝你找到对的人。", Channels: []string{"inapp"}, ReceiverRule: "payload.user_id", Closeable: true},
	{Code: "identity.UserDeactivated.v1", Module: "identity", EventCode: "identity.UserDeactivated.v1", TitleTpl: "账号已注销", ContentTpl: "你的账号已注销，相关数据将按规则处理。", Channels: []string{"inapp"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "verify.CertificationApproved.v1", Module: "verify", EventCode: "verify.CertificationApproved.v1", TitleTpl: "实名认证通过", ContentTpl: "你已通过实名认证，可发布内容与创建猫立方。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "verify.CertificationRejected.v1", Module: "verify", EventCode: "verify.CertificationRejected.v1", TitleTpl: "实名认证未通过", ContentTpl: "认证未通过，请核对信息后重新提交。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	// content
	{Code: "content.ContentSubmitted.v1", Module: "content", EventCode: "content.ContentSubmitted.v1", TitleTpl: "内容已提交审核", ContentTpl: "「{title}」已提交，审核通过后展示。", Channels: []string{"inapp"}, ReceiverRule: "payload.owner_ref.id", Closeable: true},
	{Code: "content.ContentPublished.v1", Module: "content", EventCode: "content.ContentPublished.v1", TitleTpl: "内容已通过审核", ContentTpl: "「{title}」已展示。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.owner_ref.id", Closeable: true},
	{Code: "content.ContentTakenDown.v1", Module: "content", EventCode: "content.ContentTakenDown.v1", TitleTpl: "内容已下架", ContentTpl: "「{title}」已被下架：{reason}。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.owner_ref.id", Closeable: false},
	{Code: "content.ContentClosed.v1", Module: "content", EventCode: "content.ContentClosed.v1", TitleTpl: "内容已关闭", ContentTpl: "「{title}」已关闭。", Channels: []string{"inapp"}, ReceiverRule: "payload.owner_ref.id", Closeable: true},
	// trust
	{Code: "trust.ModerationPassed.v1", Module: "trust", EventCode: "trust.ModerationPassed.v1", TitleTpl: "内容审核通过", ContentTpl: "你的内容已通过审核。", Channels: []string{"inapp"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "trust.ModerationRejected.v1", Module: "trust", EventCode: "trust.ModerationRejected.v1", TitleTpl: "内容未通过审核", ContentTpl: "内容包含违规信息（{reason_code}），请修改后重试。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "trust.ReportConfirmed.v1", Module: "trust", EventCode: "trust.ReportConfirmed.v1", TitleTpl: "举报处理结果", ContentTpl: "你对 {target_type}:{target_id} 的举报已处理。", Channels: []string{"inapp"}, ReceiverRule: "payload.reporter_id", Closeable: false},
	{Code: "trust.ViolationRecorded.v1", Module: "trust", EventCode: "trust.ViolationRecorded.v1", TitleTpl: "违规记分通知", ContentTpl: "你因违规被记 {points} 分，累计 {total} 分。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "trust.AccountRestricted.v1", Module: "trust", EventCode: "trust.AccountRestricted.v1", TitleTpl: "账号限制通知", ContentTpl: "你的账号被限制 {days} 天：{reason}。", Channels: []string{"inapp", "subscribe", "sms"}, ReceiverRule: "payload.user_id", Closeable: false},
	// social
	{Code: "social.FolloweePublished.v1", Module: "social", EventCode: "social.FolloweePublished.v1", TitleTpl: "关注的人发布了新内容", ContentTpl: "「{title}」刚刚发布。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: true},
	// trade
	{Code: "trade.OrderCreated.v1", Module: "trade", EventCode: "trade.OrderCreated.v1", TitleTpl: "订单已创建", ContentTpl: "订单金额 {amount_fen} 分，请尽快完成支付。", Channels: []string{"inapp"}, ReceiverRule: "payload.buyer_id", Closeable: false},
	{Code: "trade.OrderPaid.v1", Module: "trade", EventCode: "trade.OrderPaid.v1", TitleTpl: "支付成功", ContentTpl: "订单已支付成功。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.buyer_id", Closeable: false},
	{Code: "trade.OrderClosed.v1", Module: "trade", EventCode: "trade.OrderClosed.v1", TitleTpl: "订单已关闭", ContentTpl: "订单超时未支付已关闭。", Channels: []string{"inapp"}, ReceiverRule: "payload.buyer_id", Closeable: false},
	{Code: "trade.RefundSucceeded.v1", Module: "trade", EventCode: "trade.RefundSucceeded.v1", TitleTpl: "退款成功", ContentTpl: "退款 {amount_fen} 分已原路退回。", Channels: []string{"inapp", "sms"}, ReceiverRule: "payload.buyer_id", Closeable: false},
	{Code: "trade.ApportionFormed.v1", Module: "trade", EventCode: "trade.ApportionFormed.v1", TitleTpl: "分摊已成团", ContentTpl: "人数已满，猫立方订阅已开通。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.initiator_id", Closeable: false},
	{Code: "trade.ApportionFailed.v1", Module: "trade", EventCode: "trade.ApportionFailed.v1", TitleTpl: "分摊未成团", ContentTpl: "认筹截止未满员，已逐笔原路退款。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.initiator_id", Closeable: false},
	// membership
	{Code: "membership.MemberActivated.v1", Module: "membership", EventCode: "membership.MemberActivated.v1", TitleTpl: "会员开通成功", ContentTpl: "会员已开通，有效期至 {expires_at}。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "membership.MemberExpired.v1", Module: "membership", EventCode: "membership.MemberExpired.v1", TitleTpl: "会员已到期", ContentTpl: "会员已到期，续费继续享受权益。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "membership.SubAccountChanged.v1", Module: "membership", EventCode: "membership.SubAccountChanged.v1", TitleTpl: "子账号变更", ContentTpl: "企业版子账号发生变更（{action}）。", Channels: []string{"inapp"}, ReceiverRule: "payload.sub_user_id", Closeable: true},
	// msg
	{Code: "msg.MessageSent.v1", Module: "msg", EventCode: "msg.MessageSent.v1", TitleTpl: "新消息", ContentTpl: "你有一条新消息。", Channels: []string{"inapp"}, ReceiverRule: "payload.sender_id", Closeable: true},
	// cube
	{Code: "cube.CubeCreated.v1", Module: "cube", EventCode: "cube.CubeCreated.v1", TitleTpl: "猫立方已创建", ContentTpl: "「{name}」已创建，邀请伙伴加入吧。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.leader_id", Closeable: true},
	{Code: "cube.MemberJoined.v1", Module: "cube", EventCode: "cube.MemberJoined.v1", TitleTpl: "新成员加入", ContentTpl: "有新成员加入你的猫立方。", Channels: []string{"inapp"}, ReceiverRule: "payload.operator_id", Closeable: true},
	{Code: "cube.MemberRemoved.v1", Module: "cube", EventCode: "cube.MemberRemoved.v1", TitleTpl: "已被移出猫立方", ContentTpl: "你已被移出猫立方：{reason}。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "cube.CubeDissolved.v1", Module: "cube", EventCode: "cube.CubeDissolved.v1", TitleTpl: "猫立方已解散", ContentTpl: "「{name}」已解散。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "cube.ProjectCreated.v1", Module: "cube", EventCode: "cube.ProjectCreated.v1", TitleTpl: "新增协作项目", ContentTpl: "猫立方新增项目「{name}」。", Channels: []string{"inapp"}, ReceiverRule: "payload.operator_id", Closeable: true},
	{Code: "cube.TaskAssigned.v1", Module: "cube", EventCode: "cube.TaskAssigned.v1", TitleTpl: "新任务分配", ContentTpl: "你有一个新任务待处理。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: true},
	{Code: "cube.TaskSubmitted.v1", Module: "cube", EventCode: "cube.TaskSubmitted.v1", TitleTpl: "任务待确认", ContentTpl: "任务已提交，请确认。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.operator_id", Closeable: true},
	{Code: "cube.TaskConfirmed.v1", Module: "cube", EventCode: "cube.TaskConfirmed.v1", TitleTpl: "任务已完成", ContentTpl: "任务已确认完成。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: true},
	{Code: "cube.ReportSubmitted.v1", Module: "cube", EventCode: "cube.ReportSubmitted.v1", TitleTpl: "新报备待确认", ContentTpl: "有新的重点客户报备待确认。", Channels: []string{"inapp"}, ReceiverRule: "payload.user_id", Closeable: true},
	{Code: "cube.ReportConfirmed.v1", Module: "cube", EventCode: "cube.ReportConfirmed.v1", TitleTpl: "报备已确认", ContentTpl: "你的报备已确认。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: true},
	{Code: "cube.ReportRejected.v1", Module: "cube", EventCode: "cube.ReportRejected.v1", TitleTpl: "报备被驳回", ContentTpl: "报备被驳回：{reason}。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.user_id", Closeable: true},
	// circle
	{Code: "circle.CircleCreated.v1", Module: "circle", EventCode: "circle.CircleCreated.v1", TitleTpl: "圈子已创建", ContentTpl: "「{name}」已创建，邀请成员加入吧。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.owner_id", Closeable: true},
	{Code: "circle.CircleEnded.v1", Module: "circle", EventCode: "circle.CircleEnded.v1", TitleTpl: "圈子已到期", ContentTpl: "「{name}」已到期转为只读，可转常设。", Channels: []string{"inapp", "subscribe"}, ReceiverRule: "payload.owner_id", Closeable: false},
	{Code: "circle.CircleDissolved.v1", Module: "circle", EventCode: "circle.CircleDissolved.v1", TitleTpl: "圈子已解散", ContentTpl: "「{name}」已解散。", Channels: []string{"inapp"}, ReceiverRule: "payload.user_id", Closeable: false},
	{Code: "circle.MemberJoined.v1", Module: "circle", EventCode: "circle.MemberJoined.v1", TitleTpl: "新成员加入圈子", ContentTpl: "有新成员加入圈子。", Channels: []string{"inapp"}, ReceiverRule: "payload.user_id", Closeable: true},
	{Code: "circle.DemandPublished.v1", Module: "circle", EventCode: "circle.DemandPublished.v1", TitleTpl: "圈内新需求", ContentTpl: "圈内发布了新需求「{title}」。", Channels: []string{"inapp"}, ReceiverRule: "payload.author_id", MergeRule: domain.MergeSameCircleDay, Closeable: true},
	{Code: "circle.DemandExpired.v1", Module: "circle", EventCode: "circle.DemandExpired.v1", TitleTpl: "需求已过期", ContentTpl: "你的需求「{title}」已过期，可一键续期。", Channels: []string{"inapp"}, ReceiverRule: "payload.author_id", Closeable: true},
}
