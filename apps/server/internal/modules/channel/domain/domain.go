// Package domain channel 领域规则：状态机（#16）。
package domain

import "catcube/internal/kernel/statemachine"

// 运营商状态（与 contract.Status* 对齐）。
const (
	StateApplying  = "0" // 申请中
	StateOpened    = "1" // 已开通
	StateSuspended = "2" // 已停用
)

// MachineOperator 运营商状态机：#16 申请中→已开通→已停用（停用须重新申请）。
var MachineOperator = statemachine.Def{
	Name:   "channel.operator",
	States: []string{StateApplying, StateOpened, StateSuspended},
	Transitions: []statemachine.Transition{
		{From: StateApplying, Event: "open", To: StateOpened, Actor: "*"},
		{From: StateOpened, Event: "suspend", To: StateSuspended, Actor: "*"},
		{From: StateSuspended, Event: "reapply", To: StateApplying, Actor: "*"},
	},
}

// 结算单状态：#17 待结算→已复核→已打款（打款不可撤销）；驳回回待结算。
const (
	SettlePending  = "0" // 待结算
	SettleReviewed = "1" // 已复核
	SettlePaid     = "2" // 已打款
	SettleRejected = "3" // 已驳回
)

// MachineSettlement 月结结算单状态机（#17）：复核≠执行由内核 policy 强制。
var MachineSettlement = statemachine.Def{
	Name:     "channel.settlement",
	States:   []string{SettlePending, SettleReviewed, SettlePaid, SettleRejected},
	Terminal: []string{SettlePaid},
	Transitions: []statemachine.Transition{
		{From: SettlePending, Event: "review", To: SettleReviewed, Actor: "*"},
		{From: SettleReviewed, Event: "pay", To: SettlePaid, Actor: "*"},
		{From: SettlePending, Event: "reject", To: SettleRejected, Actor: "*"},
		{From: SettleRejected, Event: "reopen", To: SettlePending, Actor: "*"},
	},
}
