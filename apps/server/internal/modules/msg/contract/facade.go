// Package contract msg 对外契约：单聊/群聊会话、消息、增量同步。
package contract

import (
	"context"
	"time"
)

// 会话类型。
const (
	KindDM    int16 = 1 // 单聊
	KindGroup int16 = 2 // 群聊
)

// 消息类型。
const (
	MsgText  int16 = 1 // 文本
	MsgImage int16 = 2 // 图片
)

// Peer 对手方展示投影（单聊为对端用户）。
type Peer struct {
	UserID int64  `json:"user_id,string"`
	Title  string `json:"title,omitempty"`
	Cover  string `json:"cover,omitempty"`
}

// Session 会话（消息列表项）。
type Session struct {
	ID            int64      `json:"id,string"`
	Kind          int16      `json:"kind"`
	Peer          *Peer      `json:"peer,omitempty"`
	LastSeq       int64      `json:"last_seq"`
	Unread        int        `json:"unread"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	Title         string     `json:"title,omitempty"`        // 群聊名称
	LastPreview   string     `json:"last_preview,omitempty"` // 最后消息摘要
	LastSenderID  int64      `json:"last_sender_id,string,omitempty"`
}

// Message 消息。
type Message struct {
	ID        int64     `json:"id,string"`
	SessionID int64     `json:"session_id,string"`
	Seq       int64     `json:"seq"`
	SenderID  int64     `json:"sender_id,string"`
	MsgType   int16     `json:"msg_type"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// EnsureResult 发起会话结果。
type EnsureResult struct {
	Session Session `json:"session"`
	Created bool    `json:"created"`
}

// SyncResult 增量同步结果。
type SyncResult struct {
	Items      []Message `json:"items"`
	NextCursor int64     `json:"next_cursor"`
	HasMore    bool      `json:"has_more"`
}

// AdminChatSession 后台合规查看的群聊会话（含消息）。
type AdminChatSession struct {
	SessionID int64     `json:"session_id,string"`
	BizRef    string    `json:"biz_ref"`
	Title     string    `json:"title,omitempty"`
	Messages  []Message `json:"messages"`
}

// SessionMemberResolver 群聊成员解析器开口（扩展点 #3）：群聊成员由 cube 决定，msg 不认识猫立方。
type SessionMemberResolver interface {
	// Members 返回会话（biz_ref）的成员 uid 列表。
	Members(ctx context.Context, sessionRef string) ([]int64, error)
}

// Facade msg 对外接口。
type Facade interface {
	// EnsureSession 发起单聊（幂等；发起=会员权益经 policy）。
	EnsureSession(ctx context.Context, userID int64, targetRef string) (EnsureResult, error)
	// Sessions 我的会话列表（含未读数）。
	Sessions(ctx context.Context, userID int64) ([]Session, error)
	// Send 发送消息（成员校验；返回落库消息）。
	Send(ctx context.Context, userID, sessionID int64, msgType int16, content string) (Message, error)
	// Sync 增量拉取（cursor=已拉取的最大 seq）。
	Sync(ctx context.Context, userID, sessionID, cursor int64, limit int) (SyncResult, error)
	// MarkRead 提交已读游标到指定序号。
	MarkRead(ctx context.Context, userID, sessionID, readSeq int64) error
	// EnsureGroup 创建/获取群聊会话（bizRef 幂等；成员由 SessionMemberResolver 解析）。
	EnsureGroup(ctx context.Context, bizRef, title string, creatorID int64) (Session, error)
	// SyncGroups 成员变更后重算全部群聊成员（cube 成员事件驱动）。
	SyncGroups(ctx context.Context) error
	// AdminHistory 后台合规查看：按 biz_ref 列表返回群聊会话与消息（仅平台后台）。
	AdminHistory(ctx context.Context, bizRefs []string, limit int) ([]AdminChatSession, error)
}
